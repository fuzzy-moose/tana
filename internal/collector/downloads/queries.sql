-- name: GetDownload :one
SELECT * FROM panda_downloads WHERE gallery_id = ?;

-- name: ListDownloads :many
SELECT * FROM panda_downloads ORDER BY created_at DESC, gallery_id DESC LIMIT ? OFFSET ?;

-- name: RecoverDownloads :many
SELECT * FROM panda_downloads WHERE state IN ('running', 'cancelled', 'deleting');

-- name: EnqueueDownload :exec
INSERT INTO panda_downloads (gallery_id, token, state, created_at, updated_at)
VALUES (?, ?, 'queued', ?, ?) ON CONFLICT (gallery_id) DO NOTHING;

-- name: NextDownload :one
SELECT * FROM panda_downloads WHERE state = 'queued' AND retry_at <= ?
ORDER BY created_at, gallery_id LIMIT 1;

-- name: UpdateDownload :exec
UPDATE panda_downloads SET state = ?, updated_at = ?, retry_at = ?, failures = ?,
    size_bytes = ?, last_error = ? WHERE gallery_id = ?;

-- name: DeleteDownload :exec
DELETE FROM panda_downloads WHERE gallery_id = ?;
