package metadata

import (
	"context"
	"database/sql"
	"time"
)

// Reconciliation never takes the metadata dispatch lock. A durable inventory
// queue avoids revisiting unmatched imports; the cursor checks legacy entries
// once, with its upper bound fixed at migration time so new work cannot extend it.
func (s *ReferenceImports) reconcileInventory(ctx context.Context) (bool, error) {
	backfilled, err := s.reconcileExistingInventory(ctx)
	if err != nil {
		return false, err
	}
	queued, err := s.reconcileNewInventory(ctx)
	return backfilled || queued, err
}

type importInventoryEntry struct {
	id         int64
	importID   string
	token      string
	knownToken sql.NullString
}

func readImportInventoryEntries(rows *sql.Rows) ([]importInventoryEntry, error) {
	defer rows.Close()
	var entries []importInventoryEntry
	for rows.Next() {
		var entry importInventoryEntry
		if err := rows.Scan(&entry.id, &entry.importID, &entry.token, &entry.knownToken); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func settleImportInventoryEntries(ctx context.Context, tx *sql.Tx, entries []importInventoryEntry) error {
	owners := make(map[string]bool)
	for _, entry := range entries {
		if !entry.knownToken.Valid {
			continue
		}
		status := "failed"
		if entry.token == entry.knownToken.String {
			status = "known"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reference_import_entries SET status = ? WHERE id = ? AND status = 'pending'`, status, entry.id); err != nil {
			return err
		}
		owners[entry.importID] = true
	}
	// Limit completion checks to this batch's owners. Counters, completion and
	// the work checkpoint commit together, including the final pending entry.
	for id := range owners {
		if _, err := tx.ExecContext(ctx, `UPDATE reference_imports SET status = 'completed', paused = 0, completed_at = ?
			WHERE id = ? AND status = 'validating' AND pending = 0`, time.Now().UnixMilli(), id); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReferenceImports) reconcileExistingInventory(ctx context.Context) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var after, through int64
	if err := tx.QueryRowContext(ctx, `SELECT after_entry_id, through_entry_id FROM reference_import_reconciliation WHERE id = 1`).Scan(&after, &through); err != nil {
		return false, err
	}
	if after >= through {
		return false, nil
	}
	// Filter only by primary-key range before LIMIT. Filtering on inventory
	// matches would scan the entire unmatched backlog to find a single batch.
	rows, err := tx.QueryContext(ctx, `SELECT e.id, e.import_id, e.token,
		CASE WHEN e.status = 'pending' THEN (SELECT token FROM gallery_refs WHERE gallery_id = e.gallery_id) END
		FROM reference_import_entries e WHERE e.id > ? AND e.id <= ? ORDER BY e.id LIMIT ?`, after, through, importBatchRecords)
	if err != nil {
		return false, err
	}
	entries, err := readImportInventoryEntries(rows)
	if err != nil {
		return false, err
	}
	if err := settleImportInventoryEntries(ctx, tx, entries); err != nil {
		return false, err
	}
	if len(entries) < importBatchRecords {
		after = through
	} else {
		after = entries[len(entries)-1].id
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reference_import_reconciliation SET after_entry_id = ? WHERE id = 1`, after); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *ReferenceImports) reconcileNewInventory(ctx context.Context) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Bound queue reads too: metadata completion can leave queue items with no
	// pending entries. FIFO prevents later inventory from overtaking old work.
	rows, err := tx.QueryContext(ctx, `SELECT q.gallery_id, r.token FROM reference_import_inventory_queue q
		JOIN gallery_refs r ON r.gallery_id = q.gallery_id ORDER BY q.sequence LIMIT ?`, importBatchRecords)
	if err != nil {
		return false, err
	}
	type gallery struct {
		id    int64
		token string
	}
	var galleries []gallery
	for rows.Next() {
		var ref gallery
		if err := rows.Scan(&ref.id, &ref.token); err != nil {
			rows.Close()
			return false, err
		}
		galleries = append(galleries, ref)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if len(galleries) == 0 {
		return false, nil
	}
	remaining := importBatchRecords
	for _, ref := range galleries {
		rows, err := tx.QueryContext(ctx, `SELECT id, import_id, token, ? FROM reference_import_entries
			WHERE gallery_id = ? AND status = 'pending' LIMIT ?`, ref.token, ref.id, remaining)
		if err != nil {
			return false, err
		}
		batch, err := readImportInventoryEntries(rows)
		if err != nil {
			return false, err
		}
		if err := settleImportInventoryEntries(ctx, tx, batch); err != nil {
			return false, err
		}
		remaining -= len(batch)
		if _, err := tx.ExecContext(ctx, `DELETE FROM reference_import_inventory_queue WHERE gallery_id = ?
			AND NOT EXISTS (SELECT 1 FROM reference_import_entries WHERE gallery_id = ? AND status = 'pending')`, ref.id, ref.id); err != nil {
			return false, err
		}
		if remaining == 0 {
			break
		}
	}
	return true, tx.Commit()
}
