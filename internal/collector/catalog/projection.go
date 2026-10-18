package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/panda"
)

// Project updates browse fields in the transaction that retains the metadata.
// Vocabulary survives metadata changes so completion is independent of results.
func Project(ctx context.Context, tx pgx.Tx, entry panda.Metadata) error {
	english := strings.TrimSpace(html.UnescapeString(entry.Title))
	japanese := strings.TrimSpace(html.UnescapeString(entry.TitleJapanese))
	title := english
	if title == "" {
		title = japanese
	}
	category := ""
	if categories, err := panda.NormalizeCategories([]string{entry.Category}); err == nil {
		category = categories[0]
	}
	posted := time.Unix(entry.Posted, 0)
	_, err := tx.Exec(ctx, `INSERT INTO catalog_galleries
		(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, ordered_at, expunged, category, uploader)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8, $9, $10)
		ON CONFLICT (gallery_id) DO UPDATE SET title = excluded.title, title_lower = excluded.title_lower,
		title_japanese_lower = excluded.title_japanese_lower, thumbnail_url = excluded.thumbnail_url,
		page_count = excluded.page_count, posted = excluded.posted, ordered_at = excluded.ordered_at,
		expunged = excluded.expunged, category = excluded.category, metadata_source = 'collected',
		first_seen_at = NULL, feed_captured_at = NULL, feed_capture_id = NULL, uploader = excluded.uploader, description = ''`,
		entry.ID, title, strings.ToLower(english), strings.ToLower(japanese), entry.ThumbnailURL, entry.FileCount, posted, entry.Expunged, category, entry.Uploader)
	if err != nil {
		return err
	}
	return replaceTags(ctx, tx, entry.ID, entry.Tags)
}

// ProjectFeed retains the latest captured description without exposing it as
// collected metadata to enrichment. Capture order survives processing retries.
// posted and ordered_at keep whole seconds, the precision of catalog cursors.
func ProjectFeed(ctx context.Context, tx pgx.Tx, entry panda.FeedEntry, captureID int64, capturedAt, firstSeenAt time.Time) error {
	var posted *time.Time
	orderedAt := firstSeenAt.Truncate(time.Second)
	if entry.PostedAt != nil {
		orderedAt = entry.PostedAt.Truncate(time.Second)
		posted = &orderedAt
	}
	title := strings.TrimSpace(entry.Title)
	result, err := tx.Exec(ctx, `INSERT INTO catalog_galleries
		(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, posted, ordered_at,
		metadata_source, first_seen_at, feed_captured_at, feed_capture_id, uploader, description)
		SELECT $1::bigint, $2::text, $3::text, '', $4::text, $5::timestamptz, $6::timestamptz, 'feed',
		$7::timestamptz, $8::timestamptz, $9::bigint, $10::text, $11::text
		WHERE NOT EXISTS (SELECT 1 FROM gallery_metadata WHERE gallery_id = $1)
		ON CONFLICT (gallery_id) DO UPDATE SET title = excluded.title, title_lower = excluded.title_lower,
		thumbnail_url = excluded.thumbnail_url, posted = excluded.posted, ordered_at = excluded.ordered_at,
		first_seen_at = excluded.first_seen_at, feed_captured_at = excluded.feed_captured_at,
		feed_capture_id = excluded.feed_capture_id, uploader = excluded.uploader, description = excluded.description
		WHERE catalog_galleries.metadata_source = 'feed'
		AND (excluded.feed_captured_at, excluded.feed_capture_id) > (catalog_galleries.feed_captured_at, catalog_galleries.feed_capture_id)`,
		entry.GalleryRef.ID, title, strings.ToLower(title), entry.ThumbnailURL, posted, orderedAt,
		firstSeenAt, capturedAt, captureID, entry.Uploader, entry.Description)
	if err != nil {
		return err
	}
	changed := result.RowsAffected()
	// An older capture processed on retry can reveal an earlier first sighting,
	// but must not replace the newer description or its publication timestamp.
	if _, err := tx.Exec(ctx, `UPDATE catalog_galleries
		SET first_seen_at = LEAST(first_seen_at, $1),
		ordered_at = COALESCE(posted, date_trunc('second', LEAST(first_seen_at, $1)))
		WHERE gallery_id = $2 AND metadata_source = 'feed'`, firstSeenAt, entry.GalleryRef.ID); err != nil {
		return err
	}
	if changed == 0 {
		return nil
	}
	return replaceTags(ctx, tx, entry.GalleryRef.ID, entry.Tags)
}

func replaceTags(ctx context.Context, tx pgx.Tx, galleryID int64, tags []string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM catalog_gallery_tags WHERE gallery_id = $1", galleryID); err != nil {
		return err
	}
	for _, raw := range tags {
		namespace, value, found := strings.Cut(raw, ":")
		if !found {
			namespace, value = "other", raw
		}
		if namespace == "" {
			namespace = "other"
		}
		if value == "" {
			continue
		}
		namespace = strings.ToLower(namespace)
		folded := strings.ToLower(strings.ReplaceAll(value, "_", " "))
		if _, err := tx.Exec(ctx, `INSERT INTO catalog_tags (namespace, value, value_lower)
			VALUES ($1, $2, $3) ON CONFLICT (namespace, value_lower) DO NOTHING`, namespace, value, folded); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO catalog_gallery_tags (gallery_id, tag_id)
			SELECT $1::bigint, id FROM catalog_tags WHERE namespace = $2 AND value_lower = $3 ON CONFLICT DO NOTHING`, galleryID, namespace, folded); err != nil {
			return err
		}
	}
	return nil
}

// Backfill projects retained metadata in bounded transactions. Each batch locks
// its source rows FOR UPDATE, which metadata admission's upsert of the same
// rows waits on, so a concurrent update is projected after the batch, never
// overwritten by it.
// Missing projections make restart after an interrupted backfill resumable.
// Once complete, metadata writes maintain projections atomically; checking all
// retained metadata again would make every startup proportional to catalog size.
func (s *Service) Backfill(ctx context.Context) error {
	var completed bool
	if err := s.db.QueryRow(ctx, "SELECT completed FROM catalog_backfill WHERE id = 1").Scan(&completed); err != nil {
		return err
	}
	if completed {
		return nil
	}
	var after int64
	for {
		count, lastID, err := s.backfillBatch(ctx, after)
		if err != nil || count == 0 {
			return err
		}
		after = lastID
	}
}

func (s *Service) backfillBatch(ctx context.Context, after int64) (int, int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, after, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT m.gallery_id, m.body FROM gallery_metadata m
		WHERE m.gallery_id > $1 AND NOT EXISTS (SELECT 1 FROM catalog_galleries g WHERE g.gallery_id = m.gallery_id)
		ORDER BY m.gallery_id LIMIT 100 FOR UPDATE OF m`, after)
	if err != nil {
		return 0, after, err
	}
	var entries []panda.Metadata
	for rows.Next() {
		var id int64
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return 0, after, err
		}
		var entry panda.Metadata
		if err := json.Unmarshal(body, &entry); err != nil {
			rows.Close()
			return 0, after, fmt.Errorf("backfill catalog gallery %d: %w", id, err)
		}
		entry.ID = id
		entries = append(entries, entry)
		after = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, after, err
	}
	for _, entry := range entries {
		if err := Project(ctx, tx, entry); err != nil {
			return 0, after, err
		}
	}
	if len(entries) == 0 {
		if _, err := tx.Exec(ctx, "UPDATE catalog_backfill SET completed = true WHERE id = 1"); err != nil {
			return 0, after, err
		}
	}
	return len(entries), after, tx.Commit(ctx)
}
