-- name: CreateSource :one
INSERT INTO sources (library_id, path, kind) VALUES (?, ?, ?) RETURNING *;

-- name: CreateSourceFile :exec
INSERT INTO source_files (source_id, path) VALUES (?, ?);

-- name: GetSource :one
SELECT * FROM sources WHERE id = ?;

-- name: ListSources :many
SELECT * FROM sources WHERE library_id = ? ORDER BY path, id;

-- name: ListSourceFiles :many
SELECT * FROM source_files WHERE source_id = ? ORDER BY path, id;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;
