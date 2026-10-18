-- name: PendingRefs :many
-- Claims exclude reserved galleries in SQL, so they never lock a row that an
-- in-flight admission will update.
SELECT gallery_id, token FROM gallery_refs WHERE metadata_attempted_at IS NULL
AND (feed_metadata_base_at IS NULL OR feed_metadata_base_at <= sqlc.arg(now)::timestamptz -
    (SELECT make_interval(hours => feed_metadata_delay_hours) FROM metadata_collection_settings WHERE id = 1))
AND gallery_id <> ALL(sqlc.arg(reserved)::bigint[])
ORDER BY metadata_priority, gallery_id LIMIT sqlc.arg(batch_limit) FOR UPDATE OF gallery_refs SKIP LOCKED;

-- name: HasPendingRefs :one
SELECT EXISTS (SELECT 1 FROM gallery_refs WHERE metadata_attempted_at IS NULL
AND (feed_metadata_base_at IS NULL OR feed_metadata_base_at <= sqlc.arg(now)::timestamptz -
    (SELECT make_interval(hours => feed_metadata_delay_hours) FROM metadata_collection_settings WHERE id = 1)));

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, $2)
ON CONFLICT (gallery_id) DO UPDATE SET metadata_priority = 0;

-- name: SaveMetadata :exec
INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES ($1, $2, $3)
ON CONFLICT (gallery_id) DO UPDATE SET body = excluded.body, refreshed_at = excluded.refreshed_at;

-- name: RecordAttempt :exec
UPDATE gallery_refs SET metadata_attempted_at = $1, metadata_error = $2 WHERE gallery_id = $3 AND token = $4;

-- name: GetMetadata :one
SELECT body, refreshed_at FROM gallery_metadata WHERE gallery_id = $1;

-- name: LookupMetadata :one
SELECT m.body, m.refreshed_at, r.metadata_attempted_at,
    EXISTS (SELECT 1 FROM metadata_fetches f
            WHERE f.gallery_id = r.gallery_id AND f.token = r.token AND f.status = 'pending') AS pending_fetch
FROM gallery_refs r LEFT JOIN gallery_metadata m ON m.gallery_id = r.gallery_id
WHERE r.gallery_id = $1;

-- name: RetryState :one
SELECT failures, next_attempt_at FROM metadata_retry WHERE id = 1;

-- name: LockRetryState :one
SELECT failures, next_attempt_at FROM metadata_retry WHERE id = 1 FOR UPDATE;

-- name: MainBackgroundPaused :one
SELECT main_background_paused FROM metadata_collection_settings WHERE id = 1;

-- name: SetMainBackgroundPaused :exec
UPDATE metadata_collection_settings SET main_background_paused = $1 WHERE id = 1;

-- name: RecordBatchFailure :exec
UPDATE metadata_retry SET failures = $1, next_attempt_at = $2, last_attempt_at = $3, last_error = $4 WHERE id = 1;

-- name: RecordBatchSuccess :exec
UPDATE metadata_retry SET failures = 0, next_attempt_at = NULL, last_attempt_at = $1, last_error = NULL WHERE id = 1;

-- name: GetGalleryToken :one
SELECT token FROM gallery_refs WHERE gallery_id = $1;

-- name: CreateFetchJob :exec
INSERT INTO metadata_fetch_jobs (id, created_at) VALUES ($1, $2);

-- name: FindPendingFetch :one
SELECT id FROM metadata_fetches WHERE gallery_id = $1 AND token = $2 AND status = 'pending';

-- name: CreateFetch :one
INSERT INTO metadata_fetches (gallery_id, token, status, error) VALUES ($1, $2, $3, $4) RETURNING id;

-- name: AttachFetch :exec
INSERT INTO metadata_fetch_job_entries (job_id, position, fetch_id) VALUES ($1, $2, $3);

-- name: GetFetchJob :one
SELECT id, created_at, completed_at FROM metadata_fetch_jobs
WHERE id = sqlc.arg(id) AND (completed_at IS NULL OR completed_at > sqlc.arg(cutoff));

-- name: FetchJobEntries :many
SELECT f.gallery_id, f.status, f.error, f.refreshed_at
FROM metadata_fetch_job_entries e JOIN metadata_fetches f ON f.id = e.fetch_id
WHERE e.job_id = $1 ORDER BY e.position;

-- name: PendingFetches :many
SELECT f.gallery_id, f.token FROM metadata_fetches f
WHERE f.status = 'pending' AND f.gallery_id <> ALL(sqlc.arg(reserved)::bigint[]) AND NOT EXISTS (
    SELECT 1 FROM metadata_fetches older
    WHERE older.gallery_id = f.gallery_id AND older.status = 'pending' AND older.id < f.id
) ORDER BY f.id LIMIT sqlc.arg(batch_limit) FOR UPDATE OF f SKIP LOCKED;

-- name: HasPendingFetches :one
SELECT EXISTS (SELECT 1 FROM metadata_fetches WHERE status = 'pending');

-- name: FailConflictingFetches :exec
UPDATE metadata_fetches SET status = 'failed', error = 'token_conflict'
WHERE status = 'pending' AND EXISTS (
    SELECT 1 FROM gallery_refs r WHERE r.gallery_id = metadata_fetches.gallery_id AND r.token != metadata_fetches.token
);

-- name: CompleteFetch :exec
UPDATE metadata_fetches SET status = $1, error = $2, refreshed_at = $3
WHERE gallery_id = $4 AND token = $5 AND status = 'pending';

-- name: CompleteFetchJobs :exec
UPDATE metadata_fetch_jobs SET completed_at = $1
WHERE completed_at IS NULL AND NOT EXISTS (
    SELECT 1 FROM metadata_fetch_job_entries e JOIN metadata_fetches f ON f.id = e.fetch_id
    WHERE e.job_id = metadata_fetch_jobs.id AND f.status = 'pending'
);

-- name: DeleteExpiredFetchJobs :exec
DELETE FROM metadata_fetch_jobs WHERE completed_at <= $1;

-- name: DeleteOrphanFetches :exec
DELETE FROM metadata_fetches WHERE status != 'pending' AND NOT EXISTS (
    SELECT 1 FROM metadata_fetch_job_entries e WHERE e.fetch_id = metadata_fetches.id
);

-- name: SetFeedMetadataDelay :exec
UPDATE metadata_collection_settings SET feed_metadata_delay_hours = $1 WHERE id = 1;

-- name: ClearBanRetry :exec
UPDATE metadata_retry SET failures = 0, next_attempt_at = NULL
WHERE id = 1 AND strpos(last_error, sqlc.arg(ban_error)) > 0;

-- name: CountReferenceImports :one
SELECT count(*) FROM reference_imports WHERE completed_at IS NULL OR completed_at > sqlc.arg(retained_after)::timestamptz;
