-- name: BanUntil :one
SELECT until_at FROM panda_ban WHERE id = 1;

-- name: ExtendBan :exec
UPDATE panda_ban SET until_at = MAX(until_at, CAST(sqlc.arg(until_at) AS INTEGER)) WHERE id = 1;
