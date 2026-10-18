package metadata

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

const (
	MaxFetchSize = 1000
	JobRetention = 7 * 24 * time.Hour
)

var ErrInvalidBatch = errors.New("invalid metadata batch")

type FetchJob = collectorapi.MetadataFetchJob
type FetchEntry = collectorapi.MetadataFetchEntry

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
	tx, err := s.store.db.BeginTx(ctx, pgdb.ReadOnly)
	if err != nil {
		return collectorapi.LookupResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.q.WithTx(tx)
	result := collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{}, PendingIDs: []int64{}, FailedIDs: []int64{}, UnknownIDs: []int64{}}
	for _, id := range ids {
		row, err := q.LookupMetadata(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			result.UnknownIDs = append(result.UnknownIDs, id)
		} else if err != nil {
			return collectorapi.LookupResult{}, err
		} else if row.Body != nil {
			var value panda.Metadata
			if err := json.Unmarshal(row.Body, &value); err != nil {
				return collectorapi.LookupResult{}, fmt.Errorf("decode collected metadata: %w", err)
			}
			result.Galleries = append(result.Galleries, collectorapi.CollectedMetadata{Metadata: value, RefreshedAt: row.RefreshedAt.UTC()})
		} else if row.MetadataAttemptedAt == nil || row.PendingFetch {
			result.PendingIDs = append(result.PendingIDs, id)
		} else {
			result.FailedIDs = append(result.FailedIDs, id)
		}
	}
	return result, tx.Commit(ctx)
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
	tx, err := s.store.db.Begin(ctx)
	if err != nil {
		return FetchJob{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.q.WithTx(tx)
	at := time.Now()
	jobID := rand.Text()
	if err := q.CreateFetchJob(ctx, dbgen.CreateFetchJobParams{ID: jobID, CreatedAt: at}); err != nil {
		return FetchJob{}, err
	}
	for i, ref := range refs {
		token, err := q.GetGalleryToken(ctx, ref.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return FetchJob{}, err
		}
		conflict := err == nil && token != ref.Token
		fetchID, err := q.FindPendingFetch(ctx, dbgen.FindPendingFetchParams{GalleryID: ref.ID, Token: ref.Token})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return FetchJob{}, err
		}
		if errors.Is(err, pgx.ErrNoRows) || conflict {
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
	if err := q.CompleteFetchJobs(ctx, &at); err != nil {
		return FetchJob{}, err
	}
	job, err := readJob(ctx, q, jobID, at)
	if err != nil {
		return FetchJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FetchJob{}, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return job, nil
}

// GetFetchJob returns the job and its entries, or pgx.ErrNoRows for unknown or
// expired jobs. Metadata retention is independent of job retention.
func (s *Service) GetFetchJob(ctx context.Context, id string) (FetchJob, error) {
	tx, err := s.store.db.BeginTx(ctx, pgdb.ReadOnly)
	if err != nil {
		return FetchJob{}, err
	}
	defer tx.Rollback(ctx)
	job, err := readJob(ctx, s.store.q.WithTx(tx), id, time.Now())
	if err != nil {
		return FetchJob{}, err
	}
	return job, tx.Commit(ctx)
}

func readJob(ctx context.Context, q *dbgen.Queries, id string, at time.Time) (FetchJob, error) {
	row, err := q.GetFetchJob(ctx, dbgen.GetFetchJobParams{ID: id, Cutoff: new(at.Add(-JobRetention))})
	if err != nil {
		return FetchJob{}, err
	}
	entries, err := q.FetchJobEntries(ctx, id)
	if err != nil {
		return FetchJob{}, err
	}
	job := FetchJob{ID: row.ID, Status: "pending", CreatedAt: row.CreatedAt.UTC(),
		CompletedAt: pgdb.UTC(row.CompletedAt), Entries: make([]FetchEntry, len(entries))}
	if row.CompletedAt != nil {
		job.Status = "completed"
	}
	for i, entry := range entries {
		job.Entries[i] = FetchEntry{GalleryID: entry.GalleryID, Status: entry.Status,
			Error: entry.Error, RefreshedAt: pgdb.UTC(entry.RefreshedAt)}
	}
	return job, nil
}
