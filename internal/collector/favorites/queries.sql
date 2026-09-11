-- name: GetCategory :one
SELECT id, synced_at FROM favorite_categories WHERE host = ? AND account_key = ? AND category = ?;

-- name: CategoryStatistics :many
SELECT c.category, c.name, c.synced_at, count(f.gallery_id) AS favorites
FROM favorite_categories c LEFT JOIN favorites f ON f.category_id = c.id
WHERE c.host = ? AND c.account_key = ?
GROUP BY c.id ORDER BY c.category;

-- name: SaveCategory :one
INSERT INTO favorite_categories (host, account_key, category, name, synced_at) VALUES (?, ?, ?, '', 0)
ON CONFLICT (host, account_key, category) DO UPDATE SET host = excluded.host
RETURNING id;

-- name: KnownFavorites :many
SELECT gallery_id, committed_added_at FROM favorites WHERE category_id = ? AND committed_added_at IS NOT NULL;

-- name: ReconcileFavorites :exec
DELETE FROM favorites WHERE favorites.category_id = sqlc.arg(category_id) AND gallery_id NOT IN
    (SELECT gallery_id FROM favorite_sync_seen WHERE favorite_sync_seen.category_id = sqlc.arg(category_id));

-- name: CommitFavorites :exec
UPDATE favorites SET committed_added_at = added_at WHERE favorites.category_id = sqlc.arg(category_id)
    AND gallery_id IN (SELECT gallery_id FROM favorite_sync_seen WHERE favorite_sync_seen.category_id = sqlc.arg(category_id));

-- name: RenameCategory :exec
UPDATE favorite_categories SET name = ? WHERE id = ?;

-- name: CompleteCategory :exec
UPDATE favorite_categories SET synced_at = ? WHERE id = ?;

-- name: SaveFavorite :exec
INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES (?, ?, ?, ?)
ON CONFLICT (category_id, gallery_id) DO UPDATE SET token = excluded.token, added_at = excluded.added_at;

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?)
ON CONFLICT (gallery_id) DO UPDATE SET metadata_priority = 0;

-- name: GetSync :one
SELECT * FROM favorite_syncs WHERE category_id = ?;

-- name: ListSyncs :many
SELECT sqlc.embed(s), c.category FROM favorite_syncs s JOIN favorite_categories c ON c.id = s.category_id
WHERE c.host = ? AND c.account_key = ?;

-- name: NextSync :one
SELECT sqlc.embed(s), c.category FROM favorite_syncs s JOIN favorite_categories c ON c.id = s.category_id
WHERE c.host = ? AND c.account_key = ? AND (s.state IN ('queued', 'running') OR s.followup_full = 1)
ORDER BY s.state = 'running' DESC, s.queued_at, c.category LIMIT 1;

-- name: NewSync :exec
INSERT INTO favorite_syncs (category_id, state, full, queued_at) VALUES (?, 'queued', ?, ?)
ON CONFLICT (category_id) DO UPDATE SET state = 'queued', full = excluded.full,
    followup_full = 0, queued_at = excluded.queued_at, next_url = '', pages_saved = 0,
    entries_saved = 0, last_saved_at = 0, last_added_at = 0, restarted = 0,
    started_at = 0, finished_at = 0, last_error = '', last_error_at = 0, retry_at = 0, failures = 0;

-- name: QueueFullSync :exec
UPDATE favorite_syncs SET followup_full = 1 WHERE category_id = ?;

-- name: ResumeSync :exec
UPDATE favorite_syncs SET state = 'queued', finished_at = 0, retry_at = 0, failures = 0, restarted = 0 WHERE category_id = ?;

-- name: StartSync :exec
UPDATE favorite_syncs SET state = 'running', started_at = CASE WHEN started_at = 0 THEN ? ELSE started_at END WHERE category_id = ?;

-- name: SaveProgress :exec
UPDATE favorite_syncs SET next_url = sqlc.arg(next_url), pages_saved = pages_saved + 1,
    entries_saved = (SELECT count(*) FROM favorite_sync_seen WHERE favorite_sync_seen.category_id = sqlc.arg(category_id)),
    last_saved_at = sqlc.arg(last_saved_at), last_added_at = sqlc.arg(last_added_at), retry_at = 0, failures = 0 WHERE favorite_syncs.category_id = sqlc.arg(category_id);

-- name: FinishSync :exec
UPDATE favorite_syncs SET state = 'success', finished_at = ?, retry_at = 0 WHERE category_id = ?;

-- name: FailSync :exec
UPDATE favorite_syncs SET state = ?, last_error = ?, last_error_at = ?, retry_at = ?, finished_at = ?, failures = failures + 1 WHERE category_id = ?;

-- name: RestartTraversal :exec
UPDATE favorite_syncs SET next_url = '', pages_saved = 0, entries_saved = 0,
    last_added_at = 0, restarted = 1 WHERE category_id = ?;

-- name: SeenFavorites :many
SELECT gallery_id FROM favorite_sync_seen WHERE category_id = ?;

-- name: SaveSeenFavorite :exec
INSERT INTO favorite_sync_seen (category_id, gallery_id) VALUES (?, ?) ON CONFLICT DO NOTHING;

-- name: ClearSeenFavorites :exec
DELETE FROM favorite_sync_seen WHERE category_id = ?;

-- name: VisitedPages :many
SELECT url FROM favorite_sync_pages WHERE category_id = ?;

-- name: SaveVisitedPage :exec
INSERT INTO favorite_sync_pages (category_id, url) VALUES (?, ?);

-- name: ClearVisitedPages :exec
DELETE FROM favorite_sync_pages WHERE category_id = ?;

-- name: DownloadBaselineState :one
SELECT baseline_state FROM favorite_download_settings WHERE id = 1;

-- name: StartDownloadBaseline :exec
UPDATE favorite_download_settings SET baseline_state = 'collecting' WHERE id = 1;

-- name: BaselineCategories :many
SELECT category FROM favorite_download_baseline ORDER BY category;

-- name: CompleteBaselineCategory :exec
INSERT INTO favorite_download_baseline (category) VALUES (?) ON CONFLICT DO NOTHING;

-- name: FinishDownloadBaseline :exec
UPDATE favorite_download_settings SET baseline_state = 'ready'
WHERE id = 1 AND baseline_state = 'collecting' AND (SELECT count(*) FROM favorite_download_baseline) = 10;

-- name: DownloadCategories :many
SELECT category FROM favorite_download_categories ORDER BY category;

-- name: ClearDownloadCategories :exec
DELETE FROM favorite_download_categories;

-- name: EnableDownloadCategory :exec
INSERT INTO favorite_download_categories (category) VALUES (?) ON CONFLICT DO NOTHING;

-- name: ObserveFavorite :execrows
INSERT INTO favorite_observations (gallery_id) VALUES (?) ON CONFLICT DO NOTHING;

-- name: SeedFavoriteObservations :exec
INSERT INTO favorite_observations (gallery_id) SELECT DISTINCT gallery_id FROM favorites WHERE true ON CONFLICT DO NOTHING;

-- name: ResumeFailedBaselineSyncs :exec
UPDATE favorite_syncs SET state = 'queued', finished_at = 0, retry_at = 0, failures = 0, restarted = 0
WHERE state = 'failed' AND (SELECT baseline_state FROM favorite_download_settings WHERE id = 1) = 'collecting'
    AND category_id IN (SELECT id FROM favorite_categories WHERE host = ? AND account_key = ?);
