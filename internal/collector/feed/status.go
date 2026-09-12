package feed

import (
	"context"
	"database/sql"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// Refresh coalesces requests with a queued or running capture. The service owns
// the work independently of the request context once the request is accepted.
func (s *Service) Refresh(ctx context.Context) (collectorapi.FeedStatus, error) {
	if err := ctx.Err(); err != nil {
		return collectorapi.FeedStatus{}, err
	}
	s.mu.Lock()
	if !s.active {
		s.active = true
		s.refresh <- struct{}{}
	}
	s.mu.Unlock()
	return s.Status(ctx)
}

func (s *Service) Status(ctx context.Context) (collectorapi.FeedStatus, error) {
	s.mu.Lock()
	result := collectorapi.FeedStatus{CaptureActive: s.active, LastCaptureError: s.lastError, LastCaptureErrorAt: s.lastErrorAt, Continuity: "unknown"}
	s.mu.Unlock()
	var captured sql.NullInt64
	err := s.store.db.QueryRowContext(ctx, `SELECT
		(SELECT max(captured_at) FROM raw_feeds),
		(SELECT count(*) FROM raw_feeds WHERE processed_at IS NULL),
		coalesce((SELECT last_error FROM raw_feeds WHERE processed_at IS NULL AND last_error IS NOT NULL ORDER BY last_attempt_at DESC, id DESC LIMIT 1), ''),
		coalesce((SELECT c.status FROM feed_continuity_checks c JOIN raw_feeds f ON f.id = c.current_capture_id ORDER BY f.captured_at DESC, f.id DESC LIMIT 1), 'unknown'),
		(SELECT count(*) FROM feed_continuity_checks WHERE status = 'possible_gap')`).Scan(
		&captured, &result.ProcessingPending, &result.ProcessingError, &result.Continuity, &result.PossibleGaps)
	if captured.Valid {
		at := time.UnixMilli(captured.Int64).UTC()
		result.LastCapturedAt = &at
	}
	return result, err
}
