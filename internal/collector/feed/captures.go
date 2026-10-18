package feed

import (
	"context"
	"errors"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

var ErrInvalidPagination = errors.New("invalid_pagination")

func (s *Service) ListCaptures(ctx context.Context, failedOnly bool, limit, offset int64) (collectorapi.FeedCaptureList, error) {
	result := collectorapi.FeedCaptureList{Captures: []collectorapi.FeedCapture{}}
	if limit < 1 || limit > 100 || offset < 0 {
		return result, ErrInvalidPagination
	}
	// Keep the WHERE predicate equivalent to CountListedCaptures in queries.sql.
	// Only unprocessed captures still have a downloadable response body.
	// One read-only transaction reads the page and its total.
	tx, err := s.store.db.BeginTx(ctx, pgdb.ReadOnly)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id, captured_at, length(body), coalesce(last_error, '')
		FROM raw_feeds WHERE processed_at IS NULL AND (NOT $1::boolean OR last_error IS NOT NULL)
		ORDER BY captured_at DESC, id DESC LIMIT $2 OFFSET $3`, failedOnly, limit, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var capture collectorapi.FeedCapture
		if err := rows.Scan(&capture.ID, &capture.CapturedAt, &capture.SizeBytes, &capture.Error); err != nil {
			return result, err
		}
		capture.CapturedAt = capture.CapturedAt.UTC()
		capture.State = "pending"
		if capture.Error != "" {
			capture.State = "failed"
		}
		result.Captures = append(result.Captures, capture)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	if result.Total, err = s.store.q.WithTx(tx).CountListedCaptures(ctx, failedOnly); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (s *Service) CaptureBody(ctx context.Context, id int64) ([]byte, error) {
	return s.store.q.CaptureBody(ctx, id)
}
