-- name: LatestCaptureTime :one
SELECT captured_at FROM raw_feeds ORDER BY captured_at DESC, id DESC LIMIT 1;

-- name: SaveCapture :one
INSERT INTO raw_feeds (captured_at, feed_url, body) VALUES (?, ?, ?) RETURNING id;

-- name: PreviousCapture :one
SELECT id FROM raw_feeds
WHERE (captured_at, id) < (sqlc.arg(captured_at), sqlc.arg(id))
ORDER BY captured_at DESC, id DESC LIMIT 1;

-- name: NextCapture :one
SELECT id FROM raw_feeds
WHERE (captured_at, id) > (sqlc.arg(captured_at), sqlc.arg(id))
ORDER BY captured_at, id LIMIT 1;

-- name: CreateContinuityCheck :exec
INSERT INTO feed_continuity_checks
    (previous_capture_id, current_capture_id, status, created_at, checked_at)
VALUES (sqlc.arg(previous_capture_id), sqlc.arg(current_capture_id), 'unknown', sqlc.arg(now), sqlc.arg(now));

-- name: DeleteContinuityCheck :exec
DELETE FROM feed_continuity_checks WHERE previous_capture_id = ? AND current_capture_id = ?;

-- name: PendingCaptures :many
SELECT id FROM raw_feeds WHERE processed_at IS NULL ORDER BY captured_at, id;

-- name: CaptureBody :one
SELECT body FROM raw_feeds WHERE id = ?;

-- name: RecordFailure :exec
UPDATE raw_feeds SET last_attempt_at = ?, last_error = ? WHERE id = ? AND processed_at IS NULL;

-- name: SaveGalleryRef :execrows
INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?) ON CONFLICT (gallery_id) DO NOTHING;

-- name: MarkProcessed :exec
UPDATE raw_feeds SET processed_at = sqlc.arg(now), last_attempt_at = sqlc.arg(now), last_error = NULL WHERE id = sqlc.arg(id);

-- name: CompleteContinuityCheck :one
UPDATE feed_continuity_checks SET status = ?, checked_at = ?
WHERE current_capture_id = ? RETURNING *;
