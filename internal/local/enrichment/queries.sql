-- name: Enqueue :exec
INSERT INTO panda_enrichments (gallery_id, panda_id) VALUES (?, ?);

-- name: Due :many
SELECT gallery_id, panda_id, failures FROM panda_enrichments
WHERE next_attempt_at <= ? ORDER BY next_attempt_at, gallery_id LIMIT ?;

-- name: GetPending :one
SELECT panda_id FROM panda_enrichments WHERE gallery_id = ?;

-- name: Retry :exec
UPDATE panda_enrichments SET next_attempt_at = ?, failures = ? WHERE gallery_id = ?;

-- name: Complete :exec
DELETE FROM panda_enrichments WHERE gallery_id = ?;
