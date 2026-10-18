-- name: GetRun :one
SELECT * FROM sitemap_run WHERE id = 1;

-- name: LockRun :one
-- Transactions that change the run's state or claim a child lock the run first.
SELECT * FROM sitemap_run WHERE id = 1 FOR UPDATE;

-- name: DelayNextRequest :exec
UPDATE sitemap_run SET next_request_at = GREATEST(next_request_at, sqlc.arg(next_request_at)::timestamptz) WHERE id = 1 AND next_request_at IS NOT NULL;

-- name: UpdateRun :exec
UPDATE sitemap_run SET state = $1, force = $2, index_url = $3, index_ready = $4, index_failures = $5,
    started_at = $6, finished_at = $7, retry_at = $8, next_request_at = $9, last_error = $10 WHERE id = 1;

-- name: ClearChildren :exec
DELETE FROM sitemap_children;

-- name: AddChild :exec
INSERT INTO sitemap_children (url) VALUES ($1) ON CONFLICT (url) DO NOTHING;

-- name: RecoverChildren :exec
UPDATE sitemap_children SET state = 'pending' WHERE state = 'running';

-- name: RetryChildren :exec
UPDATE sitemap_children SET state = 'pending', failures = 0, retry_at = NULL, last_error = ''
WHERE state IN ('pending', 'failed', 'running');

-- name: NextChild :one
SELECT * FROM sitemap_children WHERE state = 'pending' ORDER BY retry_at NULLS FIRST, id LIMIT 1;

-- name: ClaimChild :one
SELECT * FROM sitemap_children WHERE state = 'pending' ORDER BY retry_at NULLS FIRST, id LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: HasPendingChild :one
SELECT EXISTS (SELECT 1 FROM sitemap_children WHERE state = 'pending');

-- name: StartChild :execrows
UPDATE sitemap_children SET state = 'running', references_found = 0, invalid_locations = 0
WHERE id = $1 AND state = 'pending';

-- name: UpdateChild :exec
UPDATE sitemap_children SET state = $1, failures = $2, retry_at = $3, last_error = $4 WHERE id = $5;

-- name: AddChildCounts :exec
UPDATE sitemap_children SET references_found = references_found + sqlc.arg(found),
    references_imported = references_imported + sqlc.arg(imported),
    invalid_locations = invalid_locations + sqlc.arg(invalid) WHERE id = sqlc.arg(id);

-- name: ChildCounts :one
SELECT count(*) AS total,
    count(*) FILTER (WHERE state = 'completed') AS completed,
    count(*) FILTER (WHERE state = 'skipped') AS skipped,
    count(*) FILTER (WHERE state = 'failed') AS failed,
    coalesce(sum(references_found), 0)::bigint AS found,
    coalesce(sum(references_imported), 0)::bigint AS imported,
    coalesce(sum(invalid_locations), 0)::bigint AS invalid
FROM sitemap_children;

-- name: GetValidator :one
SELECT * FROM sitemap_validators WHERE url = $1;

-- name: SaveValidator :exec
INSERT INTO sitemap_validators (url, etag, last_modified, response_url) VALUES ($1, $2, $3, $4)
ON CONFLICT (url) DO UPDATE SET etag = excluded.etag, last_modified = excluded.last_modified,
    response_url = excluded.response_url;

-- name: SaveGalleryRef :execrows
INSERT INTO gallery_refs (gallery_id, token, metadata_priority) VALUES ($1, $2, 1)
ON CONFLICT (gallery_id) DO NOTHING;

-- name: ClearChildBanRetries :exec
UPDATE sitemap_children SET retry_at = NULL WHERE state = 'pending' AND last_error = 'panda_banned';

-- name: ClearRunBanRetry :exec
-- An unset next_request_at already allows the next request immediately.
UPDATE sitemap_run SET retry_at = NULL,
    next_request_at = CASE WHEN next_request_at IS NOT NULL THEN LEAST(next_request_at, sqlc.arg(next_request_at)::timestamptz) END
WHERE id = 1 AND last_error = 'panda_banned';
