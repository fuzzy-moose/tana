package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

type persistedBatch struct {
	Batch Batch  `json:"batch"`
	Root  string `json:"root"`
}

type persistedItem struct {
	Item       Item       `json:"item"`
	Checkpoint checkpoint `json:"checkpoint"`
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func saveMetadata(ctx context.Context, db executor, b *Batch) error {
	if b.State == "completed" {
		_, err := db.ExecContext(ctx, "DELETE FROM panda_deliveries WHERE id = ?", b.ID)
		return err
	}
	b.UpdatedAt = time.Now().UTC()
	record := persistedBatch{Batch: *b, Root: b.root}
	record.Batch.Items = nil
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "UPDATE panda_deliveries SET state = ?, data = ? WHERE id = ?", b.State, data, b.ID)
	return err
}

// A batch's ordered snapshot is inserted once. Subsequent handoffs persist only
// the affected item and batch metadata, in the same transaction.
func insertItems(ctx context.Context, tx *sql.Tx, b Batch) error {
	statement, err := tx.PrepareContext(ctx, "INSERT INTO panda_delivery_items (delivery_id, position, gallery_id, state, cleanup_attempts, next_cleanup, data) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer statement.Close()
	for i, item := range b.Items {
		data, err := json.Marshal(persistedItem{Item: item, Checkpoint: item.checkpoint})
		if err != nil {
			return err
		}
		if _, err := statement.ExecContext(ctx, b.ID, i, item.GalleryID, item.State, item.CleanupAttempts, item.checkpoint.NextCleanup.UnixMilli(), data); err != nil {
			return err
		}
	}
	return nil
}

func saveItem(ctx context.Context, db executor, id int64, item Item) error {
	data, err := json.Marshal(persistedItem{Item: item, Checkpoint: item.checkpoint})
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, "UPDATE panda_delivery_items SET state = ?, cleanup_attempts = ?, next_cleanup = ?, data = ? WHERE delivery_id = ? AND gallery_id = ?",
		item.State, item.CleanupAttempts, item.checkpoint.NextCleanup.UnixMilli(), data, id, item.GalleryID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrNotFound
	}
	return err
}

func decode(data []byte) (Batch, error) {
	var record persistedBatch
	if err := json.Unmarshal(data, &record); err != nil {
		return Batch{}, err
	}
	b := record.Batch
	b.root = record.Root
	b.Items = []Item{}
	return b, nil
}

func decodeItem(data []byte) (Item, error) {
	var record persistedItem
	if err := json.Unmarshal(data, &record); err != nil {
		return Item{}, err
	}
	item := record.Item
	item.checkpoint = record.Checkpoint
	return item, nil
}

func getMetadata(ctx context.Context, db queryer, id int64) (Batch, error) {
	var data []byte
	if err := db.QueryRowContext(ctx, "SELECT data FROM panda_deliveries WHERE id = ?", id).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Batch{}, ErrNotFound
		}
		return Batch{}, err
	}
	return decode(data)
}

func (s *Service) Get(ctx context.Context, id int64) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(ctx, id)
}

func (s *Service) get(ctx context.Context, id int64) (Batch, error) {
	b, err := getMetadata(ctx, s.db, id)
	if err != nil {
		return Batch{}, err
	}
	err = s.loadItems(ctx, &b)
	return b, err
}

func (s *Service) loadItems(ctx context.Context, b *Batch) error {
	rows, err := s.db.QueryContext(ctx, "SELECT data FROM panda_delivery_items WHERE delivery_id = ? ORDER BY position", b.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		item, err := decodeItem(data)
		if err != nil {
			return err
		}
		b.Items = append(b.Items, item)
	}
	return rows.Err()
}

func (s *Service) List(ctx context.Context) ([]Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(ctx)
}

func (s *Service) list(ctx context.Context) ([]Batch, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT data FROM panda_deliveries ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batches := []Batch{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		b, err := decode(data)
		if err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range batches {
		if err := s.loadItems(ctx, &batches[i]); err != nil {
			return nil, err
		}
	}
	return batches, nil
}

func (s *Service) noActive(ctx context.Context, except int64) error {
	var active bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM panda_deliveries WHERE state IN ('running', 'paused') AND id != ?)", except).Scan(&active)
	if err == nil && active {
		return ErrActive
	}
	return err
}

func (s *Service) change(ctx context.Context, id int64, f func(*Batch) error) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return Batch{}, err
	}
	b, err := s.get(ctx, id)
	if err != nil {
		return Batch{}, err
	}
	previous := slices.Clone(b.Items)
	if err := f(&b); err != nil {
		return Batch{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback()
	for i, item := range b.Items {
		if item != previous[i] {
			if err := saveItem(ctx, tx, id, item); err != nil {
				return Batch{}, err
			}
		}
	}
	if err := saveMetadata(ctx, tx, &b); err != nil {
		return Batch{}, err
	}
	if err := tx.Commit(); err != nil {
		return Batch{}, err
	}
	s.notify()
	return b, nil
}

func (s *Service) updateBatch(ctx context.Context, id int64, f func(*Batch)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	b, err := getMetadata(ctx, s.db, id)
	if err != nil {
		return err
	}
	f(&b)
	return saveMetadata(ctx, s.db, &b)
}

func (s *Service) updateItem(b Batch, item Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(s.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := getMetadata(s.ctx, tx, b.ID)
	if err != nil {
		return err
	}
	if err := saveItem(s.ctx, tx, b.ID, item); err != nil {
		return err
	}
	if current.State != "running" {
		if err := finish(s.ctx, tx, &current); err != nil {
			return err
		}
	}
	if err := saveMetadata(s.ctx, tx, &current); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) pause(b Batch, err error) error {
	return s.updateBatch(s.ctx, b.ID, func(current *Batch) {
		// Cleanup from an old batch must not steal a newer batch's active slot.
		if current.State == "running" {
			current.State = "paused"
		}
		current.Error = err.Error()
	})
}

const unfinishedStates = "('queued', 'transferring', 'transferred', 'saved', 'importing')"

func finish(ctx context.Context, db queryer, b *Batch) error {
	var unfinished, failed bool
	err := db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM panda_delivery_items WHERE delivery_id = ? AND state IN `+unfinishedStates+`),
		EXISTS(SELECT 1 FROM panda_delivery_items WHERE delivery_id = ? AND state IN ('failed', 'cleanup_pending'))`, b.ID, b.ID).Scan(&unfinished, &failed)
	if err != nil || unfinished {
		return err
	}
	if failed {
		if b.State != "stopped" && b.State != "paused" {
			b.State = "completed_with_errors"
		}
	} else {
		b.State = "completed"
	}
	return nil
}

func (s *Service) nextItem(ctx context.Context, b Batch) (Item, error) {
	var data []byte
	var err error
	if b.CurrentGalleryID != 0 {
		err = s.db.QueryRowContext(ctx, "SELECT data FROM panda_delivery_items WHERE delivery_id = ? AND gallery_id = ?", b.ID, b.CurrentGalleryID).Scan(&data)
	} else {
		err = s.db.QueryRowContext(ctx, "SELECT data FROM panda_delivery_items WHERE delivery_id = ? AND state IN "+unfinishedStates+" ORDER BY position LIMIT 1", b.ID).Scan(&data)
	}
	if err != nil {
		return Item{}, err
	}
	return decodeItem(data)
}

func (s *Service) nextCleanup(ctx context.Context) (Batch, error) {
	var batchData, itemData []byte
	err := s.db.QueryRowContext(ctx, `SELECT d.data, i.data FROM panda_delivery_items i
		JOIN panda_deliveries d ON d.id = i.delivery_id
		WHERE d.state != 'paused' AND i.state = 'cleanup_pending' AND i.cleanup_attempts < ? AND i.next_cleanup <= ?
		ORDER BY d.id DESC, i.position LIMIT 1`, maxCleanupAttempts, time.Now().UnixMilli()).Scan(&batchData, &itemData)
	if err != nil {
		return Batch{}, err
	}
	b, err := decode(batchData)
	if err != nil {
		return Batch{}, err
	}
	item, err := decodeItem(itemData)
	b.Items = []Item{item}
	return b, err
}
