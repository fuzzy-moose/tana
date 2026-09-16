package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

// Project updates browse fields in the transaction that retains the metadata.
// Vocabulary survives metadata changes so completion is independent of results.
func Project(ctx context.Context, tx *sql.Tx, entry panda.Metadata) error {
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
	_, err := tx.ExecContext(ctx, `INSERT INTO catalog_galleries
		(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, ordered_at, expunged, category, uploader)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (gallery_id) DO UPDATE SET title = excluded.title, title_lower = excluded.title_lower,
		title_japanese_lower = excluded.title_japanese_lower, thumbnail_url = excluded.thumbnail_url,
		page_count = excluded.page_count, posted = excluded.posted, ordered_at = excluded.ordered_at,
		expunged = excluded.expunged, category = excluded.category, metadata_source = 'collected',
		first_seen_at = NULL, feed_captured_at = NULL, feed_capture_id = NULL, uploader = excluded.uploader, description = ''`,
		entry.ID, title, strings.ToLower(english), strings.ToLower(japanese), entry.ThumbnailURL, entry.FileCount, entry.Posted, entry.Posted, entry.Expunged, category, entry.Uploader)
	if err != nil {
		return err
	}
	return replaceTags(ctx, tx, entry.ID, entry.Tags)
}

// ProjectFeed retains the latest captured description without exposing it as
// collected metadata to enrichment. Capture order survives processing retries.
func ProjectFeed(ctx context.Context, tx *sql.Tx, entry panda.FeedEntry, captureID, capturedAt, firstSeenAt int64) error {
	var posted any
	orderedAt := time.UnixMilli(firstSeenAt).Unix()
	if entry.PostedAt != nil {
		posted, orderedAt = entry.PostedAt.Unix(), entry.PostedAt.Unix()
	}
	title := strings.TrimSpace(entry.Title)
	result, err := tx.ExecContext(ctx, `INSERT INTO catalog_galleries
		(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, posted, ordered_at,
		metadata_source, first_seen_at, feed_captured_at, feed_capture_id, uploader, description)
		SELECT ?, ?, ?, '', ?, ?, ?, 'feed', ?, ?, ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM gallery_metadata WHERE gallery_id = ?)
		ON CONFLICT (gallery_id) DO UPDATE SET title = excluded.title, title_lower = excluded.title_lower,
		thumbnail_url = excluded.thumbnail_url, posted = excluded.posted, ordered_at = excluded.ordered_at,
		first_seen_at = excluded.first_seen_at, feed_captured_at = excluded.feed_captured_at,
		feed_capture_id = excluded.feed_capture_id, uploader = excluded.uploader, description = excluded.description
		WHERE catalog_galleries.metadata_source = 'feed'
		AND (excluded.feed_captured_at, excluded.feed_capture_id) > (catalog_galleries.feed_captured_at, catalog_galleries.feed_capture_id)`,
		entry.GalleryRef.ID, title, strings.ToLower(title), entry.ThumbnailURL, posted, orderedAt,
		firstSeenAt, capturedAt, captureID, entry.Uploader, entry.Description, entry.GalleryRef.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// An older capture processed on retry can reveal an earlier first sighting,
	// but must not replace the newer description or its publication timestamp.
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_galleries
		SET first_seen_at = MIN(first_seen_at, ?), ordered_at = COALESCE(posted, MIN(first_seen_at, ?) / 1000)
		WHERE gallery_id = ? AND metadata_source = 'feed'`, firstSeenAt, firstSeenAt, entry.GalleryRef.ID); err != nil {
		return err
	}
	if changed == 0 {
		return nil
	}
	return replaceTags(ctx, tx, entry.GalleryRef.ID, entry.Tags)
}

func replaceTags(ctx context.Context, tx *sql.Tx, galleryID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM catalog_gallery_tags WHERE gallery_id = ?", galleryID); err != nil {
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_tags (namespace, value, value_lower)
			VALUES (?, ?, ?) ON CONFLICT (namespace, value_lower) DO NOTHING`, namespace, value, folded); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_gallery_tags (gallery_id, tag_id)
			SELECT ?, id FROM catalog_tags WHERE namespace = ? AND value_lower = ? ON CONFLICT DO NOTHING`, galleryID, namespace, folded); err != nil {
			return err
		}
	}
	return nil
}

// Backfill projects retained metadata in bounded transactions. Each batch reads
// and writes under the same lock, so concurrent metadata updates cannot be lost.
// Missing projections make restart after an interrupted backfill resumable.
// Once complete, metadata writes maintain projections atomically; checking all
// retained metadata again would make every startup proportional to catalog size.
func (s *Service) Backfill(ctx context.Context) error {
	var completed bool
	if err := s.db.QueryRowContext(ctx, "SELECT completed FROM catalog_backfill WHERE id = 1").Scan(&completed); err != nil {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, after, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT m.gallery_id, m.body FROM gallery_metadata m
		WHERE m.gallery_id > ? AND NOT EXISTS (SELECT 1 FROM catalog_galleries g WHERE g.gallery_id = m.gallery_id)
		ORDER BY m.gallery_id LIMIT 100`, after)
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
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, after, err
	}
	if err := rows.Close(); err != nil {
		return 0, after, err
	}
	for _, entry := range entries {
		if err := Project(ctx, tx, entry); err != nil {
			return 0, after, err
		}
	}
	if len(entries) == 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE catalog_backfill SET completed = 1 WHERE id = 1"); err != nil {
			return 0, after, err
		}
	}
	return len(entries), after, tx.Commit()
}
