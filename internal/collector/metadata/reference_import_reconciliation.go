package metadata

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	knownToken pgtype.Text
}

func readImportInventoryEntries(rows pgx.Rows) ([]importInventoryEntry, error) {
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

func settleImportInventoryEntries(ctx context.Context, tx pgx.Tx, entries []importInventoryEntry) error {
	owners := make(map[string]bool)
	for _, entry := range entries {
		if !entry.knownToken.Valid {
			continue
		}
		status := "failed"
		if entry.token == entry.knownToken.String {
			status = "known"
		}
		if _, err := tx.Exec(ctx, `UPDATE reference_import_entries SET status = $1 WHERE id = $2 AND status = 'pending'`, status, entry.id); err != nil {
			return err
		}
		owners[entry.importID] = true
	}
	// Limit completion checks to this batch's owners. Counters, completion and
	// the work checkpoint commit together, including the final pending entry.
	for id := range owners {
		if _, err := tx.Exec(ctx, `UPDATE reference_imports SET status = 'completed', paused = false, completed_at = $1
			WHERE id = $2 AND status = 'validating' AND pending = 0`, time.Now(), id); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReferenceImports) reconcileExistingInventory(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var after, through int64
	if err := tx.QueryRow(ctx, `SELECT after_entry_id, through_entry_id FROM reference_import_reconciliation WHERE id = 1 FOR UPDATE`).Scan(&after, &through); err != nil {
		return false, err
	}
	if after >= through {
		return false, nil
	}
	// Filter only by primary-key range before LIMIT. Filtering on inventory
	// matches would scan the entire unmatched backlog to find a single batch.
	rows, err := tx.Query(ctx, `SELECT e.id, e.import_id, e.token,
		CASE WHEN e.status = 'pending' THEN (SELECT token FROM gallery_refs WHERE gallery_id = e.gallery_id) END
		FROM reference_import_entries e WHERE e.id > $1 AND e.id <= $2 ORDER BY e.id LIMIT $3`, after, through, importBatchRecords)
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
	if _, err := tx.Exec(ctx, `UPDATE reference_import_reconciliation SET after_entry_id = $1 WHERE id = 1`, after); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type queuedInventory struct {
	id    int64
	token string
}

// claimInventoryQueue locks a bounded FIFO batch of queued galleries until tx
// ends, skipping galleries another reconciliation holds.
func claimInventoryQueue(ctx context.Context, tx pgx.Tx) ([]queuedInventory, error) {
	// Bound queue reads too: metadata completion can leave queue items with no
	// pending entries. FIFO prevents later inventory from overtaking old work.
	rows, err := tx.Query(ctx, `SELECT q.gallery_id, r.token FROM reference_import_inventory_queue q
		JOIN gallery_refs r ON r.gallery_id = q.gallery_id ORDER BY q.sequence LIMIT $1 FOR UPDATE OF q SKIP LOCKED`, importBatchRecords)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var galleries []queuedInventory
	for rows.Next() {
		var ref queuedInventory
		if err := rows.Scan(&ref.id, &ref.token); err != nil {
			return nil, err
		}
		galleries = append(galleries, ref)
	}
	return galleries, rows.Err()
}

func (s *ReferenceImports) reconcileNewInventory(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	galleries, err := claimInventoryQueue(ctx, tx)
	if err != nil {
		return false, err
	}
	if len(galleries) == 0 {
		return false, nil
	}
	remaining := importBatchRecords
	for _, ref := range galleries {
		rows, err := tx.Query(ctx, `SELECT id, import_id, token, $1::text FROM reference_import_entries
			WHERE gallery_id = $2 AND status = 'pending' LIMIT $3`, ref.token, ref.id, remaining)
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
		if _, err := tx.Exec(ctx, `DELETE FROM reference_import_inventory_queue WHERE gallery_id = $1
			AND NOT EXISTS (SELECT 1 FROM reference_import_entries WHERE gallery_id = $1 AND status = 'pending')`, ref.id); err != nil {
			return false, err
		}
		if remaining == 0 {
			break
		}
	}
	return true, tx.Commit(ctx)
}
