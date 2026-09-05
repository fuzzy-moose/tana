-- name: CreateGallery :one
INSERT INTO galleries (id, title, source_id) VALUES (?, ?, ?) RETURNING *;

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

-- name: CountGallerySummaries :one
SELECT count(*) FROM galleries WHERE instr(lower(title), lower(sqlc.arg(search))) > 0;

-- name: ListGallerySummaries :many
SELECT g.id, g.title, count(p.position) AS page_count
FROM galleries g LEFT JOIN gallery_pages p ON p.gallery_id = g.id
WHERE instr(lower(g.title), lower(sqlc.arg(search))) > 0
GROUP BY g.id
ORDER BY g.title COLLATE NOCASE, g.id
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: GetGallerySummary :one
SELECT g.id, g.title, count(p.position) AS page_count
FROM galleries g LEFT JOIN gallery_pages p ON p.gallery_id = g.id
WHERE g.id = ? GROUP BY g.id;

-- name: GetGalleryImageLocation :one
SELECT l.path AS library_path, s.path AS source_path, s.kind, f.path AS file_path
FROM gallery_pages p
JOIN source_files f ON f.id = p.source_file_id
JOIN sources s ON s.id = f.source_id
JOIN libraries l ON l.id = s.library_id
WHERE p.gallery_id = ? ORDER BY p.position LIMIT 1 OFFSET ?;
