package feed

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/feed/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db *sql.DB
	q  *dbgen.Queries
}

func newStore(db *sql.DB) *store {
	return &store{db: db, q: dbgen.New(db)}
}

func (s *store) fetchDelay(ctx context.Context, now time.Time, interval time.Duration) (time.Duration, error) {
	latest, err := s.q.LatestCaptureTime(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return max(0, time.UnixMilli(latest).Add(interval).Sub(now)), nil
}

func (s *store) capture(ctx context.Context, feedURL string, body []byte, at time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	id, err := q.SaveCapture(ctx, dbgen.SaveCaptureParams{CapturedAt: at.UnixMilli(), FeedUrl: feedURL, Body: body})
	if err != nil {
		return 0, err
	}
	previous, err := q.PreviousCapture(ctx, dbgen.PreviousCaptureParams{CapturedAt: at.UnixMilli(), ID: id})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	next, err := q.NextCapture(ctx, dbgen.NextCaptureParams{CapturedAt: at.UnixMilli(), ID: id})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	// Preserve timestamp order even if the system clock moves backwards.
	if previous != 0 && next != 0 {
		if err := q.DeleteContinuityCheck(ctx, dbgen.DeleteContinuityCheckParams{PreviousCaptureID: previous, CurrentCaptureID: next}); err != nil {
			return 0, err
		}
	}
	for _, pair := range [][2]int64{{previous, id}, {id, next}} {
		if pair[0] == 0 || pair[1] == 0 {
			continue
		}
		if err := q.CreateContinuityCheck(ctx, dbgen.CreateContinuityCheckParams{
			PreviousCaptureID: pair[0], CurrentCaptureID: pair[1], Now: at.UnixMilli(),
		}); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// complete commits references and completion together, returning newly detected gaps.
func (s *store) complete(ctx context.Context, id int64, entries []panda.FeedEntry, at time.Time) ([]dbgen.FeedContinuityCheck, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	seen := make(map[int64]struct{}, len(entries))
	overlap := false
	for _, entry := range entries {
		ref := entry.GalleryRef
		if _, duplicate := seen[ref.ID]; duplicate {
			continue
		}
		seen[ref.ID] = struct{}{}
		known, err := q.HasFeedSighting(ctx, dbgen.HasFeedSightingParams{GalleryID: ref.ID, RawFeedID: id})
		if err != nil {
			return nil, err
		}
		if known != 0 {
			overlap = true
		}
		if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: ref.ID, Token: ref.Token}); err != nil {
			return nil, err
		}
		if err := q.SaveFeedGalleryRef(ctx, dbgen.SaveFeedGalleryRefParams{RawFeedID: id, GalleryID: ref.ID}); err != nil {
			return nil, err
		}
	}
	if err := q.MarkProcessed(ctx, dbgen.MarkProcessedParams{ID: id, Now: sql.NullInt64{Int64: at.UnixMilli(), Valid: true}}); err != nil {
		return nil, err
	}
	status := "possible_gap"
	if len(seen) == 0 {
		status = "unknown"
	} else if overlap {
		status = "overlap"
	}
	check, err := q.CompleteContinuityCheck(ctx, dbgen.CompleteContinuityCheckParams{
		CurrentCaptureID: id, Status: status, CheckedAt: at.UnixMilli(),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var gaps []dbgen.FeedContinuityCheck
	if err == nil && check.Status == "possible_gap" {
		gaps = append(gaps, check)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return gaps, nil
}

func (s *store) failed(ctx context.Context, id int64, cause error) error {
	return s.q.RecordFailure(ctx, dbgen.RecordFailureParams{
		ID: id, LastAttemptAt: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
		LastError: sql.NullString{String: cause.Error(), Valid: true},
	})
}
