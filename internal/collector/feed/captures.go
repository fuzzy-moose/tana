package feed

import (
	"context"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

var ErrInvalidPagination = errors.New("invalid_pagination")

func (s *Service) ListCaptures(ctx context.Context, failedOnly bool, limit, offset int64) (collectorapi.FeedCaptureList, error) {
	result := collectorapi.FeedCaptureList{Captures: []collectorapi.FeedCapture{}}
	if limit < 1 || limit > 100 || offset < 0 {
		return result, ErrInvalidPagination
	}
	// Only unprocessed captures still have a downloadable response body.
	rows, err := s.store.db.QueryContext(ctx, `SELECT id, captured_at, length(body), coalesce(last_error, '')
		FROM raw_feeds WHERE processed_at IS NULL AND (NOT ? OR last_error IS NOT NULL)
		ORDER BY captured_at DESC, id DESC LIMIT ? OFFSET ?`, failedOnly, limit+1, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var capture collectorapi.FeedCapture
		var capturedAt int64
		if err := rows.Scan(&capture.ID, &capturedAt, &capture.SizeBytes, &capture.Error); err != nil {
			return result, err
		}
		capture.CapturedAt = time.UnixMilli(capturedAt).UTC()
		capture.State = "pending"
		if capture.Error != "" {
			capture.State = "failed"
		}
		result.Captures = append(result.Captures, capture)
	}
	if int64(len(result.Captures)) > limit {
		result.HasMore = true
		result.Captures = result.Captures[:limit]
	}
	return result, rows.Err()
}

func (s *Service) CaptureBody(ctx context.Context, id int64) ([]byte, error) {
	return s.store.q.CaptureBody(ctx, id)
}
