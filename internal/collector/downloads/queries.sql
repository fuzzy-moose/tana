-- name: GetDownload :one
SELECT * FROM panda_downloads WHERE gallery_id = $1;

-- name: ListDownloads :many
SELECT * FROM panda_downloads
WHERE sqlc.arg(state)::text = '' OR state = sqlc.arg(state)
ORDER BY created_at DESC, gallery_id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDownloadsByState :many
SELECT state, count(*) AS count FROM panda_downloads GROUP BY state;

-- name: RecoverDownloads :many
SELECT * FROM panda_downloads WHERE state IN ('running', 'cancelled', 'deleting');

-- name: EnqueueDownload :execrows
INSERT INTO panda_downloads (gallery_id, token, state, created_at, updated_at)
VALUES ($1, $2, 'queued', $3, $4) ON CONFLICT (gallery_id) DO NOTHING;

-- name: NextDownload :one
-- Locks the claimed job until the claiming transaction marks it running.
SELECT * FROM panda_downloads WHERE state = 'queued' AND (retry_at IS NULL OR retry_at <= sqlc.arg(now))
ORDER BY created_at, gallery_id LIMIT 1 FOR UPDATE SKIP LOCKED;

-- name: StartDownload :execrows
UPDATE panda_downloads SET state = 'running', updated_at = $1 WHERE gallery_id = $2 AND state = 'queued';

-- name: UpdateDownload :exec
UPDATE panda_downloads SET state = $1, updated_at = $2, retry_at = $3, failures = $4,
    size_bytes = $5, last_error = $6 WHERE gallery_id = $7;

-- name: DeleteDownload :exec
DELETE FROM panda_downloads WHERE gallery_id = $1;

-- name: GetDownloadStorage :one
SELECT * FROM panda_download_storage WHERE id = 1;

-- name: UpdateDownloadStorage :exec
UPDATE panda_download_storage SET reason = $1, archive_bytes = $2, gallery_id = $3 WHERE id = 1;

-- name: SetDownloadExpectedSize :exec
UPDATE panda_downloads SET expected_size_bytes = $1, updated_at = $2 WHERE gallery_id = $3;

-- name: ClearBanRetries :exec
UPDATE panda_downloads SET retry_at = NULL WHERE state = 'queued' AND last_error = 'panda_banned';
