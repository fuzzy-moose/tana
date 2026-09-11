-- name: GetRun :one
SELECT * FROM sitemap_run WHERE id = 1;

-- name: UpdateRun :exec
UPDATE sitemap_run SET state = ?, force = ?, index_url = ?, index_ready = ?, index_failures = ?,
    started_at = ?, finished_at = ?, retry_at = ?, next_request_at = ?, last_error = ? WHERE id = 1;

-- name: ClearChildren :exec
DELETE FROM sitemap_children;

-- name: AddChild :exec
INSERT INTO sitemap_children (url) VALUES (?) ON CONFLICT (url) DO NOTHING;

-- name: RecoverChildren :exec
UPDATE sitemap_children SET state = 'pending' WHERE state = 'running';

-- name: RetryChildren :exec
UPDATE sitemap_children SET state = 'pending', failures = 0, retry_at = 0, last_error = ''
WHERE state IN ('pending', 'failed', 'running');

-- name: NextChild :one
SELECT * FROM sitemap_children WHERE state = 'pending' ORDER BY retry_at, id LIMIT 1;

-- name: UpdateChild :exec
UPDATE sitemap_children SET state = ?, failures = ?, retry_at = ?, last_error = ? WHERE id = ?;

-- name: ResetChildCounts :exec
UPDATE sitemap_children SET references_found = 0, invalid_locations = 0 WHERE id = ?;

-- name: AddChildCounts :exec
UPDATE sitemap_children SET references_found = references_found + sqlc.arg(found),
    references_imported = references_imported + sqlc.arg(imported),
    invalid_locations = invalid_locations + sqlc.arg(invalid) WHERE id = sqlc.arg(id);

-- name: ChildCounts :one
SELECT COUNT(*) AS total,
    CAST(COALESCE(SUM(state = 'completed'), 0) AS INTEGER) AS completed,
    CAST(COALESCE(SUM(state = 'skipped'), 0) AS INTEGER) AS skipped,
    CAST(COALESCE(SUM(state = 'failed'), 0) AS INTEGER) AS failed,
    CAST(COALESCE(SUM(references_found), 0) AS INTEGER) AS found,
    CAST(COALESCE(SUM(references_imported), 0) AS INTEGER) AS imported,
    CAST(COALESCE(SUM(invalid_locations), 0) AS INTEGER) AS invalid
FROM sitemap_children;

-- name: GetValidator :one
SELECT * FROM sitemap_validators WHERE url = ?;

-- name: SaveValidator :exec
INSERT INTO sitemap_validators (url, etag, last_modified, response_url) VALUES (?, ?, ?, ?)
ON CONFLICT (url) DO UPDATE SET etag = excluded.etag, last_modified = excluded.last_modified,
    response_url = excluded.response_url;

-- name: SaveGalleryRef :execrows
INSERT INTO gallery_refs (gallery_id, token, metadata_priority) VALUES (?, ?, 1)
ON CONFLICT (gallery_id) DO NOTHING;
