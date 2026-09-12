package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"strings"

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
	_, err := tx.ExecContext(ctx, `INSERT INTO catalog_galleries
		(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, expunged)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (gallery_id) DO UPDATE SET title = excluded.title, title_lower = excluded.title_lower,
		title_japanese_lower = excluded.title_japanese_lower, thumbnail_url = excluded.thumbnail_url,
		page_count = excluded.page_count, posted = excluded.posted, expunged = excluded.expunged`,
		entry.ID, title, strings.ToLower(english), strings.ToLower(japanese), entry.ThumbnailURL, entry.FileCount, entry.Posted, entry.Expunged)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM catalog_gallery_tags WHERE gallery_id = ?", entry.ID); err != nil {
		return err
	}
	for _, raw := range entry.Tags {
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
			SELECT ?, id FROM catalog_tags WHERE namespace = ? AND value_lower = ? ON CONFLICT DO NOTHING`, entry.ID, namespace, folded); err != nil {
			return err
		}
	}
	return nil
}

// Backfill projects retained metadata in bounded transactions. Each batch reads
// and writes under the same lock, so concurrent metadata updates cannot be lost.
// Missing projections make restart after an interrupted backfill resumable.
func (s *Service) Backfill(ctx context.Context) error {
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
	return len(entries), after, tx.Commit()
}
