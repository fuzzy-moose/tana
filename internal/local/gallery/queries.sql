-- name: CreateGallery :one
INSERT INTO galleries (title, source_id) VALUES ($1, $2) RETURNING *;

-- name: GetGallery :one
SELECT * FROM galleries WHERE id = $1;

-- name: LockGallery :one
-- Serializes page replacements of one gallery.
SELECT * FROM galleries WHERE id = $1 FOR UPDATE;

-- name: ListGalleries :many
SELECT * FROM galleries ORDER BY title, id;

-- name: RenameGallery :one
UPDATE galleries SET title = $1 WHERE id = $2 RETURNING *;

-- name: DeleteGallery :exec
DELETE FROM galleries WHERE id = $1;

-- name: CreateGalleryPage :exec
INSERT INTO gallery_pages (gallery_id, position, source_file_id) VALUES ($1, $2, $3);

-- name: DeleteGalleryPages :exec
DELETE FROM gallery_pages WHERE gallery_id = $1;

-- name: ListGalleryPages :many
SELECT gallery_id, source_file_id,
       row_number() OVER (ORDER BY position) AS page_number
FROM gallery_pages WHERE gallery_id = $1 ORDER BY position;

-- name: GetSourceFilePath :one
SELECT path FROM source_files WHERE id = $1;

-- name: GetSourceTitle :one
SELECT sources.path, sources.kind, libraries.path AS library_path
FROM sources JOIN libraries ON libraries.id = sources.library_id
WHERE sources.id = $1;

-- name: ListSourceFilesForGallery :many
SELECT id, path FROM source_files WHERE source_id = $1;

-- name: GetGallerySummary :one
SELECT g.id, g.title, count(p.position) AS page_count
FROM galleries g LEFT JOIN gallery_pages p ON p.gallery_id = g.id
WHERE g.id = $1 GROUP BY g.id;

-- name: GetGalleryImageLocation :one
SELECT l.path AS library_path, s.path AS source_path, s.kind, f.path AS file_path
FROM gallery_pages p
JOIN source_files f ON f.id = p.source_file_id
JOIN sources s ON s.id = f.source_id
JOIN libraries l ON l.id = s.library_id
WHERE p.gallery_id = $1 ORDER BY p.position LIMIT 1 OFFSET $2;
