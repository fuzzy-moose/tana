-- name: CreateSource :one
INSERT INTO sources (library_id, path, kind) VALUES ($1, $2, $3) RETURNING *;

-- name: CreateSourceFile :exec
INSERT INTO source_files (source_id, path) VALUES ($1, $2);

-- name: GetSource :one
SELECT * FROM sources WHERE id = $1;

-- name: ListSources :many
SELECT * FROM sources WHERE library_id = $1 ORDER BY path, id;

-- name: ListSourceFiles :many
SELECT * FROM source_files WHERE source_id = $1 ORDER BY path, id;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = $1;
