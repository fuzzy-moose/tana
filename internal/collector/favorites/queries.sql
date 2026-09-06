-- name: GetCategory :one
SELECT id FROM favorite_categories WHERE host = ? AND account_key = ? AND category = ?;

-- name: CategoryStatistics :many
SELECT c.category, c.name, c.synced_at, count(f.gallery_id) AS favorites
FROM favorite_categories c LEFT JOIN favorites f ON f.category_id = c.id
WHERE c.host = ? AND c.account_key = ?
GROUP BY c.id ORDER BY c.category;

-- name: SaveCategory :one
INSERT INTO favorite_categories (host, account_key, category, name, synced_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (host, account_key, category) DO UPDATE SET name = excluded.name, synced_at = excluded.synced_at
RETURNING id;

-- name: KnownFavorites :many
SELECT gallery_id, added_at FROM favorites WHERE category_id = ?;

-- name: DeleteFavorites :exec
DELETE FROM favorites WHERE category_id = ?;

-- name: SaveFavorite :exec
INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES (?, ?, ?, ?)
ON CONFLICT (category_id, gallery_id) DO UPDATE SET token = excluded.token, added_at = excluded.added_at;

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?) ON CONFLICT (gallery_id) DO NOTHING;
