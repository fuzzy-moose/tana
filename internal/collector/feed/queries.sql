-- name: LatestCaptureTime :one
SELECT captured_at FROM raw_feeds ORDER BY captured_at DESC, id DESC LIMIT 1;

-- name: SaveCapture :one
INSERT INTO raw_feeds (captured_at, feed_url, body) VALUES ($1, $2, $3) RETURNING id;

-- name: PreviousCapture :one
SELECT id FROM raw_feeds
WHERE (captured_at, id) < (sqlc.arg(captured_at)::timestamptz, sqlc.arg(id)::bigint)
ORDER BY captured_at DESC, id DESC LIMIT 1;

-- name: NextCapture :one
SELECT id FROM raw_feeds
WHERE (captured_at, id) > (sqlc.arg(captured_at)::timestamptz, sqlc.arg(id)::bigint)
ORDER BY captured_at, id LIMIT 1;

-- name: CreateContinuityCheck :exec
INSERT INTO feed_continuity_checks
    (previous_capture_id, current_capture_id, status, created_at, checked_at)
VALUES (sqlc.arg(previous_capture_id), sqlc.arg(current_capture_id), 'unknown', sqlc.arg(now), sqlc.arg(now));

-- name: DeleteContinuityCheck :exec
DELETE FROM feed_continuity_checks WHERE previous_capture_id = $1 AND current_capture_id = $2;

-- name: PendingCaptures :many
SELECT id FROM raw_feeds WHERE processed_at IS NULL ORDER BY captured_at, id;

-- name: CaptureBody :one
SELECT body FROM raw_feeds WHERE id = $1 AND processed_at IS NULL;

-- name: RecordFailure :exec
UPDATE raw_feeds SET last_attempt_at = $1, last_error = $2 WHERE id = $3 AND processed_at IS NULL;

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, $2)
ON CONFLICT (gallery_id) DO UPDATE SET metadata_priority = 0;

-- name: HasFeedSighting :one
SELECT EXISTS(SELECT 1 FROM feed_gallery_refs WHERE gallery_id = $1 AND raw_feed_id != $2);

-- name: SaveFeedGalleryRef :exec
INSERT INTO feed_gallery_refs (raw_feed_id, gallery_id) VALUES ($1, $2)
ON CONFLICT (raw_feed_id, gallery_id) DO NOTHING;

-- name: MarkProcessed :exec
UPDATE raw_feeds SET processed_at = sqlc.arg(now), last_attempt_at = sqlc.arg(now), last_error = NULL, body = ''::bytea WHERE id = sqlc.arg(id);

-- name: CompleteContinuityCheck :one
UPDATE feed_continuity_checks SET status = $1, checked_at = $2
WHERE current_capture_id = $3 RETURNING *;

-- name: CountListedCaptures :one
-- Keep the WHERE predicate equivalent to the inline list query in captures.go.
SELECT count(*) FROM raw_feeds
WHERE processed_at IS NULL AND (NOT sqlc.arg(failed_only)::boolean OR last_error IS NOT NULL);
