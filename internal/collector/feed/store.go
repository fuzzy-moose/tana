package feed

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/feed/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db *pgxpool.Pool
	q  *dbgen.Queries
}

func newStore(db *pgxpool.Pool) *store {
	return &store{db: db, q: dbgen.New(db)}
}

func (s *store) fetchDelay(ctx context.Context, now time.Time, interval time.Duration) (time.Duration, error) {
	latest, err := s.q.LatestCaptureTime(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return max(0, latest.Add(interval).Sub(now)), nil
}

func (s *store) capture(ctx context.Context, feedURL string, body []byte, at time.Time) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	id, err := q.SaveCapture(ctx, dbgen.SaveCaptureParams{CapturedAt: at, FeedUrl: feedURL, Body: body})
	if err != nil {
		return 0, err
	}
	previous, err := q.PreviousCapture(ctx, dbgen.PreviousCaptureParams{CapturedAt: at, ID: id})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	next, err := q.NextCapture(ctx, dbgen.NextCaptureParams{CapturedAt: at, ID: id})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
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
			PreviousCaptureID: pair[0], CurrentCaptureID: pair[1], Now: at,
		}); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit(ctx)
}

// complete commits references and completion together, returning newly detected gaps.
func (s *store) complete(ctx context.Context, id int64, entries []panda.FeedEntry, at time.Time) ([]dbgen.FeedContinuityCheck, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	var capturedAt time.Time
	if err := tx.QueryRow(ctx, "SELECT captured_at FROM raw_feeds WHERE id = $1 AND processed_at IS NULL", id).Scan(&capturedAt); err != nil {
		return nil, err
	}
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
		if known {
			overlap = true
		}
		if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: ref.ID, Token: ref.Token}); err != nil {
			return nil, err
		}
		if err := q.SaveFeedGalleryRef(ctx, dbgen.SaveFeedGalleryRefParams{RawFeedID: id, GalleryID: ref.ID}); err != nil {
			return nil, err
		}
		var firstSeenAt time.Time
		if err := tx.QueryRow(ctx, `SELECT MIN(f.captured_at) FROM feed_gallery_refs s
			JOIN raw_feeds f ON f.id = s.raw_feed_id WHERE s.gallery_id = $1`, ref.ID).Scan(&firstSeenAt); err != nil {
			return nil, err
		}
		baseAt := firstSeenAt
		if entry.PostedAt != nil {
			baseAt = *entry.PostedAt
		}
		// Retries may reveal an earlier sighting or publication time. They can
		// shorten the wait, but repeated sightings must never postpone it.
		// LEAST ignores a NULL base.
		if _, err := tx.Exec(ctx, `UPDATE gallery_refs
			SET feed_metadata_base_at = LEAST(feed_metadata_base_at, $1)
			WHERE gallery_id = $2`, baseAt, ref.ID); err != nil {
			return nil, err
		}
		if err := catalog.ProjectFeed(ctx, tx, entry, id, capturedAt, firstSeenAt); err != nil {
			return nil, err
		}
	}
	if err := q.MarkProcessed(ctx, dbgen.MarkProcessedParams{ID: id, Now: &at}); err != nil {
		return nil, err
	}
	status := "possible_gap"
	if len(seen) == 0 {
		status = "unknown"
	} else if overlap {
		status = "overlap"
	}
	check, err := q.CompleteContinuityCheck(ctx, dbgen.CompleteContinuityCheckParams{
		CurrentCaptureID: id, Status: status, CheckedAt: at,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var gaps []dbgen.FeedContinuityCheck
	if err == nil && check.Status == "possible_gap" {
		gaps = append(gaps, check)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return gaps, nil
}

func (s *store) failed(ctx context.Context, id int64, cause error) error {
	now := time.Now()
	return s.q.RecordFailure(ctx, dbgen.RecordFailureParams{
		ID: id, LastAttemptAt: &now, LastError: pgtype.Text{String: cause.Error(), Valid: true},
	})
}
