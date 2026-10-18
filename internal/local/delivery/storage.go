package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuzzy-moose/tana/internal/pgdb"
)

type persistedBatch struct {
	Batch Batch  `json:"batch"`
	Root  string `json:"root"`
}

type persistedItem struct {
	Item       Item       `json:"item"`
	Checkpoint checkpoint `json:"checkpoint"`
}

// dbtx is satisfied by the pool and by transactions.
type dbtx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func saveMetadata(ctx context.Context, db dbtx, b *Batch) error {
	if b.State == "completed" {
		_, err := db.Exec(ctx, "DELETE FROM panda_deliveries WHERE id = $1", b.ID)
		return err
	}
	b.UpdatedAt = time.Now().UTC()
	record := persistedBatch{Batch: *b, Root: b.root}
	record.Batch.Items = nil
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, "UPDATE panda_deliveries SET state = $1, data = $2 WHERE id = $3", b.State, data, b.ID)
	return err
}

// A batch's ordered snapshot is inserted once. Subsequent handoffs persist only
// the affected item and batch metadata, in the same transaction.
func insertItems(ctx context.Context, tx pgx.Tx, b Batch) error {
	for i, item := range b.Items {
		data, err := json.Marshal(persistedItem{Item: item, Checkpoint: item.checkpoint})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO panda_delivery_items (delivery_id, position, gallery_id, state, cleanup_attempts, next_cleanup, data) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			b.ID, i, item.GalleryID, item.State, item.CleanupAttempts, pgdb.NullTime(item.checkpoint.NextCleanup), data); err != nil {
			return err
		}
	}
	return nil
}

// errItemChanged reports an item that another transaction holds or moved out of
// the state a transition expected.
var errItemChanged = errors.New("library delivery item changed concurrently")

// saveItem stores item only while its persisted state is still from.
func saveItem(ctx context.Context, db dbtx, id int64, from string, item Item) error {
	data, err := json.Marshal(persistedItem{Item: item, Checkpoint: item.checkpoint})
	if err != nil {
		return err
	}
	result, err := db.Exec(ctx, `UPDATE panda_delivery_items SET state = $1, cleanup_attempts = $2, next_cleanup = $3, data = $4
		WHERE delivery_id = $5 AND gallery_id = $6 AND state = $7`,
		item.State, item.CleanupAttempts, pgdb.NullTime(item.checkpoint.NextCleanup), data, id, item.GalleryID, from)
	if err == nil && result.RowsAffected() != 1 {
		return errItemChanged
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

func getMetadata(ctx context.Context, db dbtx, id int64) (Batch, error) {
	return readMetadata(ctx, db, "SELECT data FROM panda_deliveries WHERE id = $1", id)
}

// lockMetadata reads a batch and locks its row until tx ends. Transactions that
// write a batch's items lock the batch first.
func lockMetadata(ctx context.Context, tx pgx.Tx, id int64) (Batch, error) {
	return readMetadata(ctx, tx, "SELECT data FROM panda_deliveries WHERE id = $1 FOR UPDATE", id)
}

func readMetadata(ctx context.Context, db dbtx, query string, id int64) (Batch, error) {
	var data []byte
	if err := db.QueryRow(ctx, query, id).Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	err = loadItems(ctx, s.db, &b)
	return b, err
}

func loadItems(ctx context.Context, db dbtx, b *Batch) error {
	rows, err := db.Query(ctx, "SELECT data FROM panda_delivery_items WHERE delivery_id = $1 ORDER BY position", b.ID)
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
	rows, err := s.db.Query(ctx, "SELECT data FROM panda_deliveries ORDER BY id DESC")
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
	for i := range batches {
		if err := loadItems(ctx, s.db, &batches[i]); err != nil {
			return nil, err
		}
	}
	return batches, nil
}

func (s *Service) noActive(ctx context.Context, except int64) error {
	var active bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM panda_deliveries WHERE state IN ('running', 'paused') AND id != $1)", except).Scan(&active)
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx)
	b, err := lockMetadata(ctx, tx, id)
	if err != nil {
		return Batch{}, err
	}
	if err := loadItems(ctx, tx, &b); err != nil {
		return Batch{}, err
	}
	previous := slices.Clone(b.Items)
	if err := f(&b); err != nil {
		return Batch{}, err
	}
	for i, item := range b.Items {
		if item != previous[i] {
			if err := saveItem(ctx, tx, id, previous[i].State, item); err != nil {
				return Batch{}, err
			}
		}
	}
	if err := saveMetadata(ctx, tx, &b); err != nil {
		return Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	b, err := lockMetadata(ctx, tx, id)
	if err != nil {
		return err
	}
	f(&b)
	if err := saveMetadata(ctx, tx, &b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// updateItem stores item, expecting its persisted state to be from.
func (s *Service) updateItem(b Batch, from string, item Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	tx, err := s.db.Begin(s.ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(s.ctx)
	current, err := lockMetadata(s.ctx, tx, b.ID)
	if err != nil {
		return err
	}
	if err := saveItem(s.ctx, tx, b.ID, from, item); err != nil {
		return err
	}
	return s.commitItem(tx, current)
}

// recordAttempt persists a cleanup attempt as the item's claim: only while the
// item is still pending with the attempts this worker read, and not while
// another transaction holds it. A lost claim reports false and writes nothing.
func (s *Service) recordAttempt(b Batch, item Item) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return false, err
	}
	tx, err := s.db.Begin(s.ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(s.ctx)
	current, err := lockMetadata(s.ctx, tx, b.ID)
	if err != nil {
		return false, err
	}
	var attempts int
	err = tx.QueryRow(s.ctx, `SELECT cleanup_attempts FROM panda_delivery_items
		WHERE delivery_id = $1 AND gallery_id = $2 AND state = 'cleanup_pending'
		FOR UPDATE SKIP LOCKED`, b.ID, item.GalleryID).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && attempts != item.CleanupAttempts-1 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := saveItem(s.ctx, tx, b.ID, "cleanup_pending", item); err != nil {
		return false, err
	}
	return true, s.commitItem(tx, current)
}

// commitItem settles the locked batch after one of its items changed.
func (s *Service) commitItem(tx pgx.Tx, current Batch) error {
	if current.State != "running" {
		if err := finish(s.ctx, tx, &current); err != nil {
			return err
		}
	}
	if err := saveMetadata(s.ctx, tx, &current); err != nil {
		return err
	}
	return tx.Commit(s.ctx)
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

func finish(ctx context.Context, db dbtx, b *Batch) error {
	var unfinished, failed bool
	err := db.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM panda_delivery_items WHERE delivery_id = $1 AND state IN `+unfinishedStates+`),
		EXISTS(SELECT 1 FROM panda_delivery_items WHERE delivery_id = $1 AND state IN ('failed', 'cleanup_pending'))`, b.ID).Scan(&unfinished, &failed)
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

// lockNextItem locks the batch's current item, or else its first unfinished
// one. pgx.ErrNoRows means the batch has no such item; errItemChanged means
// another transaction holds it, and the claim never skips ahead of it.
func lockNextItem(ctx context.Context, tx pgx.Tx, b Batch) (Item, error) {
	galleryID := b.CurrentGalleryID
	var err error
	if galleryID != 0 {
		err = tx.QueryRow(ctx, "SELECT gallery_id FROM panda_delivery_items WHERE delivery_id = $1 AND gallery_id = $2", b.ID, galleryID).Scan(&galleryID)
	} else {
		err = tx.QueryRow(ctx, "SELECT gallery_id FROM panda_delivery_items WHERE delivery_id = $1 AND state IN "+unfinishedStates+" ORDER BY position LIMIT 1", b.ID).Scan(&galleryID)
	}
	if err != nil {
		return Item{}, err
	}
	var data []byte
	err = tx.QueryRow(ctx, "SELECT data FROM panda_delivery_items WHERE delivery_id = $1 AND gallery_id = $2 FOR UPDATE SKIP LOCKED", b.ID, galleryID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, errItemChanged
	}
	if err != nil {
		return Item{}, err
	}
	return decodeItem(data)
}

func (s *Service) nextCleanup(ctx context.Context) (Batch, error) {
	var batchData, itemData []byte
	err := s.db.QueryRow(ctx, `SELECT d.data, i.data FROM panda_delivery_items i
		JOIN panda_deliveries d ON d.id = i.delivery_id
		WHERE d.state != 'paused' AND i.state = 'cleanup_pending' AND i.cleanup_attempts < $1
		  AND (i.next_cleanup IS NULL OR i.next_cleanup <= $2)
		ORDER BY d.id DESC, i.position LIMIT 1`, maxCleanupAttempts, time.Now()).Scan(&batchData, &itemData)
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
