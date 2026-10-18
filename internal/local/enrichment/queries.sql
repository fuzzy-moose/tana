-- name: Enqueue :exec
INSERT INTO panda_enrichments (gallery_id, panda_id) VALUES ($1, $2);

-- name: Due :many
SELECT gallery_id, panda_id, failures FROM panda_enrichments
WHERE next_attempt_at IS NULL OR next_attempt_at <= sqlc.arg(now)::timestamptz
ORDER BY next_attempt_at NULLS FIRST, gallery_id LIMIT sqlc.arg(row_limit);

-- name: ClaimDue :many
-- Locks the listed enrichments that are still due until the claiming
-- transaction completes or reschedules them. Rows locked by another
-- transaction, rescheduled, or deleted since Due are left out.
SELECT gallery_id, panda_id, failures FROM panda_enrichments
WHERE gallery_id = ANY(sqlc.arg(gallery_ids)::bigint[])
  AND (next_attempt_at IS NULL OR next_attempt_at <= sqlc.arg(now)::timestamptz)
ORDER BY gallery_id FOR UPDATE SKIP LOCKED;

-- name: Retry :exec
UPDATE panda_enrichments SET next_attempt_at = $1, failures = $2 WHERE gallery_id = $3;

-- name: Complete :exec
DELETE FROM panda_enrichments WHERE gallery_id = $1;
