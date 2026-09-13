-- name: GetDownload :one
SELECT * FROM panda_downloads WHERE gallery_id = ?;

-- name: ListDownloads :many
SELECT * FROM panda_downloads
WHERE CAST(sqlc.arg(state) AS TEXT) = '' OR state = sqlc.arg(state)
ORDER BY created_at DESC, gallery_id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDownloadsByState :many
SELECT state, count(*) AS count FROM panda_downloads GROUP BY state;

-- name: RecoverDownloads :many
SELECT * FROM panda_downloads WHERE state IN ('running', 'cancelled', 'deleting');

-- name: EnqueueDownload :execrows
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

-- name: GetDownloadStorage :one
SELECT * FROM panda_download_storage WHERE id = 1;

-- name: UpdateDownloadStorage :exec
UPDATE panda_download_storage SET reason = ?, archive_bytes = ?, gallery_id = ? WHERE id = 1;

-- name: SetDownloadExpectedSize :exec
UPDATE panda_downloads SET expected_size_bytes = ?, updated_at = ? WHERE gallery_id = ?;
