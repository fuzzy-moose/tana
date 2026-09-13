package metadata

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

// pendingImportRefs chooses at most one token per gallery from the oldest
// outstanding import. Claimable inventory takes priority in proxy channels.
func (s *store) pendingImportRefs(ctx context.Context, reserved map[int64]bool) ([]panda.GalleryRef, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT e.id, e.gallery_id, e.token FROM reference_import_entries e
		WHERE e.import_id = (SELECT id FROM reference_imports WHERE status IN ('processing', 'validating') AND paused = 0 ORDER BY sequence LIMIT 1)
		AND e.status = 'pending' AND NOT EXISTS (
			SELECT 1 FROM reference_import_entries older
			WHERE older.import_id = e.import_id AND older.gallery_id = e.gallery_id AND older.status = 'pending' AND older.id < e.id
		) ORDER BY e.id LIMIT ?`, panda.MaxBatchSize+len(reserved))
	if err != nil {
		return nil, err
	}
	refs := []panda.GalleryRef{}
	entryIDs := make(map[int64]int64)
	for rows.Next() {
		var id int64
		var ref panda.GalleryRef
		if err := rows.Scan(&id, &ref.ID, &ref.Token); err != nil {
			rows.Close()
			return nil, err
		}
		if !reserved[ref.ID] && len(refs) < panda.MaxBatchSize {
			refs = append(refs, ref)
			entryIDs[ref.ID] = id
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Check only this bounded batch. Scanning every pending import entry on
	// every upstream request would make large imports quadratic.
	pending := refs[:0]
	for _, ref := range refs {
		var token string
		err := tx.QueryRowContext(ctx, `SELECT token FROM gallery_refs WHERE gallery_id = ?`, ref.ID).Scan(&token)
		if errors.Is(err, sql.ErrNoRows) {
			pending = append(pending, ref)
		} else if err != nil {
			return nil, err
		} else {
			// Only settle selected entries under the dispatch lock. The import
			// worker reconciles other owners and conflicting tokens separately.
			if _, err := tx.ExecContext(ctx, `UPDATE reference_import_entries SET status = CASE WHEN token = ? THEN 'known' ELSE 'failed' END
				WHERE id = ? AND status = 'pending'`, token, entryIDs[ref.ID]); err != nil {
				return nil, err
			}
		}
	}
	if err := completeReferenceImports(ctx, tx, time.Now()); err != nil {
		return nil, err
	}
	return pending, tx.Commit()
}

// Complete all current owners, including imports admitted while the request
// was in flight. Terminal outcomes in cancelled or completed owners are never
// changed. This runs inside the metadata-admission transaction.
func completeImportedReference(ctx context.Context, tx *sql.Tx, ref panda.GalleryRef, successful, known bool) error {
	status := "failed"
	if known {
		status = "known"
	} else if successful {
		status = "imported"
	}
	_, err := tx.ExecContext(ctx, `UPDATE reference_import_entries SET status = ?
		WHERE gallery_id = ? AND token = ? AND status = 'pending'`, status, ref.ID, ref.Token)
	return err
}

func settleImportedGallery(ctx context.Context, tx *sql.Tx, galleryID int64, token string) error {
	// Conflicting-token fanout must not turn a metadata response into a large
	// reconciliation sweep. The durable inventory queue finishes the remainder.
	_, err := tx.ExecContext(ctx, `UPDATE reference_import_entries SET status = CASE WHEN token = ? THEN 'known' ELSE 'failed' END
		WHERE id IN (SELECT id FROM reference_import_entries WHERE gallery_id = ? AND status = 'pending' LIMIT ?)`, token, galleryID, importBatchRecords)
	return err
}
