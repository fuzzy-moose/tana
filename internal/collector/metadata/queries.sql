-- name: PendingRefs :many
SELECT gallery_id, token FROM gallery_refs WHERE metadata_attempted_at IS NULL ORDER BY gallery_id LIMIT ?;

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?) ON CONFLICT (gallery_id) DO NOTHING;

-- name: SaveMetadata :exec
INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (?, ?, ?)
ON CONFLICT (gallery_id) DO UPDATE SET body = excluded.body, refreshed_at = excluded.refreshed_at;

-- name: RecordAttempt :exec
UPDATE gallery_refs SET metadata_attempted_at = ?, metadata_error = ? WHERE gallery_id = ? AND token = ?;

-- name: GetMetadata :one
SELECT body, refreshed_at FROM gallery_metadata WHERE gallery_id = ?;

-- name: LookupMetadata :one
SELECT m.body, m.refreshed_at, r.metadata_attempted_at,
    EXISTS (SELECT 1 FROM metadata_fetches f
            WHERE f.gallery_id = r.gallery_id AND f.token = r.token AND f.status = 'pending') AS pending_fetch
FROM gallery_refs r LEFT JOIN gallery_metadata m ON m.gallery_id = r.gallery_id
WHERE r.gallery_id = ?;

-- name: RetryState :one
SELECT failures, next_attempt_at FROM metadata_retry WHERE id = 1;

-- name: RecordBatchFailure :exec
UPDATE metadata_retry SET failures = ?, next_attempt_at = ?, last_attempt_at = ?, last_error = ? WHERE id = 1;

-- name: RecordBatchSuccess :exec
UPDATE metadata_retry SET failures = 0, next_attempt_at = 0, last_attempt_at = ?, last_error = NULL WHERE id = 1;

-- name: GetGalleryToken :one
SELECT token FROM gallery_refs WHERE gallery_id = ?;

-- name: CreateFetchJob :exec
INSERT INTO metadata_fetch_jobs (id, created_at) VALUES (?, ?);

-- name: FindPendingFetch :one
SELECT id FROM metadata_fetches WHERE gallery_id = ? AND token = ? AND status = 'pending';

-- name: CreateFetch :one
INSERT INTO metadata_fetches (gallery_id, token, status, error) VALUES (?, ?, ?, ?) RETURNING id;

-- name: AttachFetch :exec
INSERT INTO metadata_fetch_job_entries (job_id, position, fetch_id) VALUES (?, ?, ?);

-- name: GetFetchJob :one
SELECT id, created_at, completed_at FROM metadata_fetch_jobs
WHERE id = ? AND (completed_at IS NULL OR completed_at > sqlc.arg(cutoff));

-- name: FetchJobEntries :many
SELECT f.gallery_id, f.status, f.error, f.refreshed_at
FROM metadata_fetch_job_entries e JOIN metadata_fetches f ON f.id = e.fetch_id
WHERE e.job_id = ? ORDER BY e.position;

-- name: PendingFetches :many
SELECT f.gallery_id, f.token FROM metadata_fetches f
WHERE f.status = 'pending' AND NOT EXISTS (
    SELECT 1 FROM metadata_fetches older
    WHERE older.gallery_id = f.gallery_id AND older.status = 'pending' AND older.id < f.id
) ORDER BY f.id LIMIT ?;

-- name: FailConflictingFetches :exec
UPDATE metadata_fetches SET status = 'failed', error = 'token_conflict'
WHERE status = 'pending' AND EXISTS (
    SELECT 1 FROM gallery_refs r WHERE r.gallery_id = metadata_fetches.gallery_id AND r.token != metadata_fetches.token
);

-- name: CompleteFetch :exec
UPDATE metadata_fetches SET status = ?, error = ?, refreshed_at = ?
WHERE gallery_id = ? AND token = ? AND status = 'pending';

-- name: CompleteFetchJobs :exec
UPDATE metadata_fetch_jobs SET completed_at = ?
WHERE completed_at IS NULL AND NOT EXISTS (
    SELECT 1 FROM metadata_fetch_job_entries e JOIN metadata_fetches f ON f.id = e.fetch_id
    WHERE e.job_id = metadata_fetch_jobs.id AND f.status = 'pending'
);

-- name: DeleteExpiredFetchJobs :exec
DELETE FROM metadata_fetch_jobs WHERE completed_at <= ?;

-- name: DeleteOrphanFetches :exec
DELETE FROM metadata_fetches WHERE status != 'pending' AND NOT EXISTS (
    SELECT 1 FROM metadata_fetch_job_entries e WHERE e.fetch_id = metadata_fetches.id
);
