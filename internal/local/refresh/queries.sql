-- name: ListSources :many
SELECT s.id, s.library_id, s.path, s.kind,
       l.name AS library_name, l.path AS library_path
FROM sources s JOIN libraries l ON l.id = s.library_id
WHERE s.library_id = sqlc.arg(library_id) OR sqlc.arg(library_id) = 0
ORDER BY l.id, s.path, s.id;

-- name: ListAffectedGalleries :many
SELECT g.id AS id, g.title, 1 AS deleted,
       (SELECT count(*) FROM gallery_pages p WHERE p.gallery_id = g.id) AS pages_removed
FROM sources s JOIN galleries g ON g.source_id = s.id
WHERE s.id = sqlc.arg(source_id)
UNION ALL
SELECT g.id AS id, g.title, 0 AS deleted, count(*) AS pages_removed
FROM gallery_pages p
JOIN source_files f ON f.id = p.source_file_id
JOIN galleries g ON g.id = p.gallery_id
WHERE f.source_id = sqlc.arg(source_id)
  AND (g.source_id IS NULL OR g.source_id != sqlc.arg(source_id))
GROUP BY g.id
ORDER BY id;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;
