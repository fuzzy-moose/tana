package metadata

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

const (
	MaxFetchSize = 1000
	JobRetention = 7 * 24 * time.Hour
)

var ErrInvalidBatch = errors.New("invalid metadata batch")

type FetchJob struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
	Entries     []FetchEntry `json:"entries"`
}

type FetchEntry struct {
	GalleryID   int64      `json:"gid"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	RefreshedAt *time.Time `json:"refreshed_at,omitempty"`
}

func validateIDs(ids []int64, limit int) error {
	if len(ids) == 0 || len(ids) > limit {
		return fmt.Errorf("%w: expected 1–%d galleries", ErrInvalidBatch, limit)
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("%w: gallery IDs must be positive and unique", ErrInvalidBatch)
		}
		seen[id] = true
	}
	return nil
}

// Lookup reads retained metadata without scheduling upstream work.
func (s *Service) Lookup(ctx context.Context, ids []int64) (collectorapi.LookupResult, error) {
	if err := validateIDs(ids, collectorapi.MaxLookupSize); err != nil {
		return collectorapi.LookupResult{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return collectorapi.LookupResult{}, err
	}
	defer tx.Rollback()
	q := s.store.q.WithTx(tx)
	result := collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{}, PendingIDs: []int64{}, FailedIDs: []int64{}, UnknownIDs: []int64{}}
	for _, id := range ids {
		row, err := q.LookupMetadata(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			result.UnknownIDs = append(result.UnknownIDs, id)
		} else if err != nil {
			return collectorapi.LookupResult{}, err
		} else if row.Body != nil {
			var value panda.Metadata
			if err := json.Unmarshal(row.Body, &value); err != nil {
				return collectorapi.LookupResult{}, fmt.Errorf("decode collected metadata: %w", err)
			}
			result.Galleries = append(result.Galleries, collectorapi.CollectedMetadata{Metadata: value, RefreshedAt: time.UnixMilli(row.RefreshedAt.Int64).UTC()})
		} else if !row.MetadataAttemptedAt.Valid || row.PendingFetch != 0 {
			result.PendingIDs = append(result.PendingIDs, id)
		} else {
			result.FailedIDs = append(result.FailedIDs, id)
		}
	}
	return result, tx.Commit()
}

// RequestFetch commits a job that survives subsequent request cancellation.
// Only pending work is shared; completed work never suppresses a fresh request.
func (s *Service) RequestFetch(ctx context.Context, refs []panda.GalleryRef) (FetchJob, error) {
	ids := make([]int64, len(refs))
	for i, ref := range refs {
		ids[i] = ref.ID
		if ref.Token == "" || strings.IndexFunc(ref.Token, unicode.IsSpace) >= 0 {
			return FetchJob{}, fmt.Errorf("%w: tokens must be nonempty and contain no whitespace", ErrInvalidBatch)
		}
	}
	if err := validateIDs(ids, MaxFetchSize); err != nil {
		return FetchJob{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return FetchJob{}, err
	}
	defer tx.Rollback()
	q := s.store.q.WithTx(tx)
	at := time.Now()
	jobID := rand.Text()
	if err := q.CreateFetchJob(ctx, dbgen.CreateFetchJobParams{ID: jobID, CreatedAt: at.UnixMilli()}); err != nil {
		return FetchJob{}, err
	}
	for i, ref := range refs {
		token, err := q.GetGalleryToken(ctx, ref.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return FetchJob{}, err
		}
		conflict := err == nil && token != ref.Token
		fetchID, err := q.FindPendingFetch(ctx, dbgen.FindPendingFetchParams{GalleryID: ref.ID, Token: ref.Token})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return FetchJob{}, err
		}
		if errors.Is(err, sql.ErrNoRows) || conflict {
			args := dbgen.CreateFetchParams{GalleryID: ref.ID, Token: ref.Token, Status: "pending"}
			if conflict {
				args.Status, args.Error = "failed", "token_conflict"
			}
			fetchID, err = q.CreateFetch(ctx, args)
			if err != nil {
				return FetchJob{}, err
			}
		}
		if err := q.AttachFetch(ctx, dbgen.AttachFetchParams{JobID: jobID, Position: int64(i), FetchID: fetchID}); err != nil {
			return FetchJob{}, err
		}
	}
	if err := q.CompleteFetchJobs(ctx, nullableMillis(at)); err != nil {
		return FetchJob{}, err
	}
	job, err := readJob(ctx, q, jobID, at)
	if err != nil {
		return FetchJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return FetchJob{}, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return job, nil
}

// GetFetchJob returns a consistent snapshot, or sql.ErrNoRows for unknown or
// expired jobs. Metadata retention is independent of job retention.
func (s *Service) GetFetchJob(ctx context.Context, id string) (FetchJob, error) {
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return FetchJob{}, err
	}
	defer tx.Rollback()
	job, err := readJob(ctx, s.store.q.WithTx(tx), id, time.Now())
	if err != nil {
		return FetchJob{}, err
	}
	return job, tx.Commit()
}

func readJob(ctx context.Context, q *dbgen.Queries, id string, at time.Time) (FetchJob, error) {
	row, err := q.GetFetchJob(ctx, dbgen.GetFetchJobParams{ID: id, Cutoff: nullableMillis(at.Add(-JobRetention))})
	if err != nil {
		return FetchJob{}, err
	}
	entries, err := q.FetchJobEntries(ctx, id)
	if err != nil {
		return FetchJob{}, err
	}
	job := FetchJob{ID: row.ID, Status: "pending", CreatedAt: time.UnixMilli(row.CreatedAt).UTC(),
		CompletedAt: optionalTime(row.CompletedAt), Entries: make([]FetchEntry, len(entries))}
	if row.CompletedAt.Valid {
		job.Status = "completed"
	}
	for i, entry := range entries {
		job.Entries[i] = FetchEntry{GalleryID: entry.GalleryID, Status: entry.Status,
			Error: entry.Error, RefreshedAt: optionalTime(entry.RefreshedAt)}
	}
	return job, nil
}

func nullableMillis(at time.Time) sql.NullInt64 {
	return sql.NullInt64{Int64: at.UnixMilli(), Valid: true}
}

func optionalTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	at := time.UnixMilli(value.Int64).UTC()
	return &at
}
