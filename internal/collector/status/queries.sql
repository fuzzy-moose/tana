-- name: InventoryStatistics :one
WITH inventory AS (
    SELECT CASE WHEN m.gallery_id IS NOT NULL THEN 'available'
        WHEN r.metadata_attempted_at IS NULL OR EXISTS (
            SELECT 1 FROM metadata_fetches f
            WHERE f.gallery_id = r.gallery_id AND f.token = r.token AND f.status = 'pending'
        ) THEN 'pending' ELSE 'failed' END AS state
    FROM gallery_refs r LEFT JOIN gallery_metadata m ON m.gallery_id = r.gallery_id
)
SELECT count(*) AS gallery_references,
    count(CASE WHEN state = 'available' THEN 1 END) AS metadata_available,
    count(CASE WHEN state = 'pending' THEN 1 END) AS metadata_pending,
    count(CASE WHEN state = 'failed' THEN 1 END) AS metadata_failed,
    (SELECT count(*) FROM metadata_fetches WHERE status = 'pending') AS fetches_pending,
    (SELECT count(*) FROM metadata_fetches WHERE status = 'failed') AS fetches_failed
FROM inventory;

-- name: MetadataRetry :one
SELECT next_attempt_at, last_error FROM metadata_retry WHERE id = 1;

-- name: RecentMetadataErrors :many
WITH errors AS (
    SELECT gallery_id, metadata_error, metadata_attempted_at
    FROM gallery_refs WHERE metadata_error IS NOT NULL AND metadata_error != ''
    UNION ALL
    SELECT f.gallery_id, f.error, (
        SELECT max(coalesce(j.completed_at, j.created_at))
        FROM metadata_fetch_job_entries e JOIN metadata_fetch_jobs j ON j.id = e.job_id
        WHERE e.fetch_id = f.id
    ) AS metadata_attempted_at
    FROM metadata_fetches f WHERE f.status = 'failed'
), latest AS (
    SELECT *, row_number() OVER (PARTITION BY gallery_id ORDER BY metadata_attempted_at DESC) AS position
    FROM errors
)
SELECT gallery_id, metadata_error, metadata_attempted_at FROM latest WHERE position = 1
ORDER BY metadata_attempted_at DESC, gallery_id LIMIT 10;
