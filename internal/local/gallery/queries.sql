-- name: CreateGallery :one
INSERT INTO galleries (id, title) VALUES (?, ?) RETURNING *;

-- name: GetGallery :one
SELECT * FROM galleries WHERE id = ?;

-- name: ListGalleries :many
SELECT * FROM galleries ORDER BY title, id;

-- name: RenameGallery :one
UPDATE galleries SET title = ? WHERE id = ? RETURNING *;

-- name: DeleteGallery :exec
DELETE FROM galleries WHERE id = ?;

-- name: CreateGalleryPage :exec
INSERT INTO gallery_pages (gallery_id, position, source_file_id) VALUES (?, ?, ?);

-- name: DeleteGalleryPages :exec
DELETE FROM gallery_pages WHERE gallery_id = ?;

-- name: ListGalleryPages :many
SELECT gallery_id, source_file_id,
       CAST(row_number() OVER (ORDER BY position) AS INTEGER) AS page_number
FROM gallery_pages WHERE gallery_id = ? ORDER BY position;

-- name: GetSourceFilePath :one
SELECT path FROM source_files WHERE id = ?;

-- name: GetSourceTitle :one
SELECT sources.path, sources.kind, libraries.path AS library_path
FROM sources JOIN libraries ON libraries.id = sources.library_id
WHERE sources.id = ?;

-- name: ListSourceFilesForGallery :many
SELECT id, path FROM source_files WHERE source_id = ?;
