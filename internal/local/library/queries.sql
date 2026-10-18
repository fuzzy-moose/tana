-- name: CreateLibrary :one
INSERT INTO libraries (name, path, availability, last_checked_at)
VALUES ($1, $2, 'available', $3)
RETURNING *;

-- name: LockLibraries :exec
-- Serializes root validation against concurrent registrations and other
-- library writes; reads proceed.
LOCK TABLE libraries IN SHARE ROW EXCLUSIVE MODE;

-- name: ListLibraries :many
SELECT * FROM libraries ORDER BY name, id;

-- name: GetLibrary :one
SELECT * FROM libraries WHERE id = $1;

-- name: RenameLibrary :one
UPDATE libraries SET name = $1 WHERE id = $2 RETURNING *;

-- name: DeleteLibrary :exec
DELETE FROM libraries WHERE id = $1;

-- name: UpdateAvailability :exec
UPDATE libraries SET availability = $1, last_checked_at = $2 WHERE id = $3;

-- name: ResetAvailability :exec
UPDATE libraries SET availability = 'unknown', last_checked_at = NULL;
