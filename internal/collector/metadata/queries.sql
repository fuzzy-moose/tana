-- name: PendingRefs :many
SELECT gallery_id, token FROM gallery_refs WHERE metadata_attempted_at IS NULL ORDER BY gallery_id LIMIT ?;

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?) ON CONFLICT (gallery_id) DO NOTHING;

-- name: SaveMetadata :exec
INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (?, ?, ?)
ON CONFLICT (gallery_id) DO UPDATE SET body = excluded.body, refreshed_at = excluded.refreshed_at;

-- name: RecordAttempt :exec
UPDATE gallery_refs SET metadata_attempted_at = ?, metadata_error = ? WHERE gallery_id = ?;

-- name: GetMetadata :one
SELECT body, refreshed_at FROM gallery_metadata WHERE gallery_id = ?;

-- name: RetryState :one
SELECT failures, next_attempt_at FROM metadata_retry WHERE id = 1;

-- name: RecordBatchFailure :exec
UPDATE metadata_retry SET failures = ?, next_attempt_at = ?, last_attempt_at = ?, last_error = ? WHERE id = 1;

-- name: RecordBatchSuccess :exec
UPDATE metadata_retry SET failures = 0, next_attempt_at = 0, last_attempt_at = ?, last_error = NULL WHERE id = 1;
