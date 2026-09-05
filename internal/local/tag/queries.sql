-- name: EnsureNamespace :one
INSERT INTO namespaces (id, name) VALUES (?, ?)
ON CONFLICT (name) DO UPDATE SET name = excluded.name RETURNING *;

-- name: EnsureTag :one
INSERT INTO tags (id, namespace_id, value) VALUES (?, ?, ?)
ON CONFLICT (namespace_id, value) DO UPDATE SET value = excluded.value RETURNING *;

-- name: AssignGalleryTag :exec
INSERT INTO gallery_tags (gallery_id, tag_id) VALUES (?, ?)
ON CONFLICT DO NOTHING;

-- name: DeleteGalleryTags :exec
DELETE FROM gallery_tags WHERE gallery_id = ?;

-- name: ListGalleryTags :many
SELECT t.id, t.value, n.id AS namespace_id, n.name AS namespace_name
FROM gallery_tags g JOIN tags t ON t.id = g.tag_id
JOIN namespaces n ON n.id = t.namespace_id
WHERE g.gallery_id = ? ORDER BY n.name, t.value;
