-- name: CreateLibrary :one
INSERT INTO libraries (name, path, availability, last_checked_at)
VALUES (?, ?, 'available', ?)
RETURNING *;

-- name: ListLibraries :many
SELECT * FROM libraries ORDER BY name, id;

-- name: GetLibrary :one
SELECT * FROM libraries WHERE id = ?;

-- name: RenameLibrary :one
UPDATE libraries SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteLibrary :exec
DELETE FROM libraries WHERE id = ?;

-- name: UpdateAvailability :exec
UPDATE libraries SET availability = ?, last_checked_at = ? WHERE id = ?;

-- name: ResetAvailability :exec
UPDATE libraries SET availability = 'unknown', last_checked_at = NULL;
