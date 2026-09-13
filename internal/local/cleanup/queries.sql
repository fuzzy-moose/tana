-- name: ListSources :many
SELECT s.id, s.library_id, s.path, s.kind,
       l.name AS library_name, l.path AS library_path,
       EXISTS (
           SELECT 1 FROM source_files f
           JOIN gallery_pages p ON p.source_file_id = f.id
           JOIN galleries g ON g.id = p.gallery_id
           WHERE f.source_id = s.id AND (g.source_id IS NULL OR g.source_id != s.id)
       ) AS protected
FROM sources s JOIN libraries l ON l.id = s.library_id
ORDER BY l.name, s.path, s.id;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;
