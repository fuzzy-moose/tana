-- name: BanUntil :one
SELECT until_at FROM panda_ban WHERE id = $1;

-- name: ExtendBan :exec
UPDATE panda_ban SET until_at = GREATEST(until_at, sqlc.arg(until_at)::timestamptz) WHERE id = sqlc.arg(id);

-- name: ResetBan :exec
UPDATE panda_ban SET until_at = NULL WHERE id = $1;
