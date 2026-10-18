-- name: GetCategory :one
SELECT id, synced_at FROM favorite_categories WHERE host = $1 AND account_key = $2 AND category = $3;

-- name: CategoryStatistics :many
SELECT c.category, c.name, c.synced_at, count(f.gallery_id) AS favorites
FROM favorite_categories c LEFT JOIN favorites f ON f.category_id = c.id
WHERE c.host = $1 AND c.account_key = $2
GROUP BY c.id ORDER BY c.category;

-- name: SaveCategory :one
INSERT INTO favorite_categories (host, account_key, category, name, synced_at) VALUES ($1, $2, $3, '', NULL)
ON CONFLICT (host, account_key, category) DO UPDATE SET host = excluded.host
RETURNING id;

-- name: KnownFavorites :many
SELECT gallery_id, committed_added_at FROM favorites WHERE category_id = $1 AND committed_added_at IS NOT NULL;

-- name: DownloadCandidates :many
SELECT f.gallery_id, f.token, CAST(coalesce(d.state, '') AS TEXT) AS state
FROM favorites f
JOIN favorite_categories c ON c.id = f.category_id
LEFT JOIN panda_downloads d ON d.gallery_id = f.gallery_id
WHERE c.host = $1 AND c.account_key = $2 AND c.category = $3
ORDER BY f.gallery_id;

-- name: ReconcileFavorites :exec
DELETE FROM favorites WHERE favorites.category_id = sqlc.arg(category_id) AND gallery_id NOT IN
    (SELECT gallery_id FROM favorite_sync_seen WHERE favorite_sync_seen.category_id = sqlc.arg(category_id));

-- name: CommitFavorites :exec
UPDATE favorites SET committed_added_at = added_at WHERE favorites.category_id = sqlc.arg(category_id)
    AND gallery_id IN (SELECT gallery_id FROM favorite_sync_seen WHERE favorite_sync_seen.category_id = sqlc.arg(category_id));

-- name: RenameCategory :exec
UPDATE favorite_categories SET name = $1 WHERE id = $2;

-- name: CompleteCategory :exec
UPDATE favorite_categories SET synced_at = $1 WHERE id = $2;

-- name: SaveFavorite :exec
INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (category_id, gallery_id) DO UPDATE SET token = excluded.token, added_at = excluded.added_at;

-- name: SaveGalleryRef :exec
INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, $2)
ON CONFLICT (gallery_id) DO UPDATE SET metadata_priority = 0;

-- name: LockSync :one
SELECT * FROM favorite_syncs WHERE category_id = $1 FOR UPDATE;

-- name: ListSyncs :many
SELECT sqlc.embed(s), c.category FROM favorite_syncs s JOIN favorite_categories c ON c.id = s.category_id
WHERE c.host = $1 AND c.account_key = $2;

-- name: NextSync :one
SELECT sqlc.embed(s), c.category FROM favorite_syncs s JOIN favorite_categories c ON c.id = s.category_id
WHERE c.host = $1 AND c.account_key = $2 AND (s.state IN ('queued', 'running') OR s.followup_full)
ORDER BY s.state = 'running' DESC, s.queued_at, c.category LIMIT 1
FOR UPDATE OF s SKIP LOCKED;

-- name: NewSync :exec
INSERT INTO favorite_syncs (category_id, state, "full", queued_at) VALUES ($1, 'queued', $2, $3)
ON CONFLICT (category_id) DO UPDATE SET state = 'queued', "full" = excluded."full",
    followup_full = false, queued_at = excluded.queued_at, next_url = '', pages_saved = 0,
    entries_saved = 0, last_saved_at = NULL, last_added_at = NULL, restarted = false,
    started_at = NULL, finished_at = NULL, last_error = '', last_error_at = NULL, retry_at = NULL, failures = 0;

-- name: QueueFullSync :exec
UPDATE favorite_syncs SET followup_full = true WHERE category_id = $1;

-- name: ResumeSync :exec
UPDATE favorite_syncs SET state = 'queued', finished_at = NULL, retry_at = NULL, failures = 0, restarted = false WHERE category_id = $1;

-- name: StartSync :one
UPDATE favorite_syncs SET state = 'running', started_at = coalesce(started_at, sqlc.arg(started_at)::timestamptz)
WHERE category_id = sqlc.arg(category_id) AND state IN ('queued', 'running')
RETURNING *;

-- name: SaveProgress :exec
UPDATE favorite_syncs SET next_url = sqlc.arg(next_url), pages_saved = pages_saved + 1,
    entries_saved = (SELECT count(*) FROM favorite_sync_seen WHERE favorite_sync_seen.category_id = sqlc.arg(category_id)),
    last_saved_at = sqlc.arg(last_saved_at), last_added_at = sqlc.arg(last_added_at), retry_at = NULL, failures = 0 WHERE favorite_syncs.category_id = sqlc.arg(category_id);

-- name: FinishSync :exec
UPDATE favorite_syncs SET state = 'success', finished_at = $1, retry_at = NULL WHERE category_id = $2;

-- name: FailSync :exec
UPDATE favorite_syncs SET state = $1, last_error = $2, last_error_at = $3, retry_at = $4, finished_at = $5, failures = failures + 1
WHERE category_id = $6 AND state = 'running';

-- name: RestartTraversal :exec
UPDATE favorite_syncs SET next_url = '', pages_saved = 0, entries_saved = 0,
    last_added_at = NULL, restarted = true WHERE category_id = $1;

-- name: SeenFavorites :many
SELECT gallery_id FROM favorite_sync_seen WHERE category_id = $1;

-- name: SaveSeenFavorite :exec
INSERT INTO favorite_sync_seen (category_id, gallery_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ClearSeenFavorites :exec
DELETE FROM favorite_sync_seen WHERE category_id = $1;

-- name: VisitedPages :many
SELECT url FROM favorite_sync_pages WHERE category_id = $1;

-- name: SaveVisitedPage :exec
INSERT INTO favorite_sync_pages (category_id, url) VALUES ($1, $2);

-- name: ClearVisitedPages :exec
DELETE FROM favorite_sync_pages WHERE category_id = $1;

-- name: DownloadBaselineState :one
SELECT baseline_state FROM favorite_download_settings WHERE id = 1;

-- name: LockDownloadSettings :one
-- Transactions that lock both rows take the settings row before favorite_syncs.
SELECT baseline_state FROM favorite_download_settings WHERE id = 1 FOR UPDATE;

-- name: StartDownloadBaseline :exec
UPDATE favorite_download_settings SET baseline_state = 'collecting' WHERE id = 1;

-- name: BaselineCategories :many
SELECT category FROM favorite_download_baseline ORDER BY category;

-- name: CompleteBaselineCategory :exec
INSERT INTO favorite_download_baseline (category) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: FinishDownloadBaseline :exec
UPDATE favorite_download_settings SET baseline_state = 'ready'
WHERE id = 1 AND baseline_state = 'collecting' AND (SELECT count(*) FROM favorite_download_baseline) = 10;

-- name: DownloadCategories :many
SELECT category FROM favorite_download_categories ORDER BY category;

-- name: ClearDownloadCategories :exec
DELETE FROM favorite_download_categories;

-- name: EnableDownloadCategory :exec
INSERT INTO favorite_download_categories (category) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: ObserveFavorite :execrows
INSERT INTO favorite_observations (gallery_id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: SeedFavoriteObservations :exec
INSERT INTO favorite_observations (gallery_id) SELECT DISTINCT gallery_id FROM favorites WHERE true ON CONFLICT DO NOTHING;

-- name: ResumeFailedBaselineSyncs :exec
UPDATE favorite_syncs SET state = 'queued', finished_at = NULL, retry_at = NULL, failures = 0, restarted = false
WHERE state = 'failed' AND (SELECT baseline_state FROM favorite_download_settings WHERE id = 1) = 'collecting'
    AND category_id IN (SELECT id FROM favorite_categories WHERE host = $1 AND account_key = $2);

-- name: ClearBanRetries :exec
UPDATE favorite_syncs SET retry_at = NULL WHERE state = 'running' AND strpos(last_error, sqlc.arg(ban_error)) > 0;
