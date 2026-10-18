// Package delivery durably transfers retained collector archives into libraries.
package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/local/library"
)

var (
	ErrActive      = errors.New("a library delivery batch is already active")
	ErrNotFound    = errors.New("library delivery batch not found")
	ErrInvalid     = errors.New("invalid library delivery request")
	ErrUnavailable = errors.New("destination library is unavailable")
)

const maxCleanupAttempts = 3

type collector interface {
	OpenDownload(context.Context, int64, string, http.Header) (*http.Response, error)
	DeleteDownload(context.Context, int64) error
}

type libraryCatalog interface {
	Get(context.Context, int64) (library.Library, error)
}

type importer interface {
	// ImportArchive accepts a path relative to the library root and is idempotent.
	ImportArchive(context.Context, int64, string) error
}

type Item struct {
	GalleryID       int64  `json:"gallery_id"`
	State           string `json:"state"`
	Error           string `json:"error,omitempty"`
	CleanupAttempts int    `json:"cleanup_attempts"`
	checkpoint      checkpoint
}

type Batch struct {
	ID               int64     `json:"id"`
	LibraryID        int64     `json:"library_id"`
	State            string    `json:"state"`
	Items            []Item    `json:"items"`
	Error            string    `json:"error,omitempty"`
	StopRequested    bool      `json:"stop_requested"`
	CurrentGalleryID int64     `json:"current_gallery_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	root             string
}

// Checkpoints distinguish a failed operation from the last durable handoff.
// The staging link establishes file ownership even if finalization is interrupted.
type checkpoint struct {
	Token       string    `json:"token"`
	Stage       string    `json:"stage"`
	SHA256      string    `json:"sha256,omitempty"`
	Size        int64     `json:"size"`
	NextCleanup time.Time `json:"next_cleanup"`
}

type Service struct {
	db        *pgxpool.Pool
	collector collector
	libraries libraryCatalog
	importer  importer
	logger    *slog.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	wake      chan struct{}
	wg        sync.WaitGroup
	probeOnce sync.Once
	probe     *library.DirectoryProbe
	// Serializes persisted batch updates; never held during transfers or imports.
	mu sync.Mutex
}

// New resumes interrupted work. Paused batches require an explicit Resume.
func New(ctx context.Context, db *pgxpool.Pool, client collector, libraries libraryCatalog, imports importer, logger *slog.Logger) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, collector: client, libraries: libraries, importer: imports, logger: logger, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	if _, err := s.List(ctx); err != nil {
		cancel()
		return nil, err
	}
	s.wg.Go(s.run)
	s.notify()
	return s, nil
}

func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

func (s *Service) Start(ctx context.Context, libraryID int64, galleryIDs []int64) (Batch, error) {
	if libraryID <= 0 || len(galleryIDs) == 0 {
		return Batch{}, ErrInvalid
	}
	lib, err := s.libraries.Get(ctx, libraryID)
	if err != nil {
		return Batch{}, err
	}
	b := Batch{LibraryID: libraryID, State: "running", Items: []Item{}, root: lib.Path, CreatedAt: time.Now().UTC()}
	seen := map[int64]bool{}
	for _, id := range galleryIDs {
		if id <= 0 {
			return Batch{}, ErrInvalid
		}
		if !seen[id] {
			b.Items = append(b.Items, Item{GalleryID: id, State: "queued", checkpoint: checkpoint{Stage: "queued", Token: newToken()}})
			seen[id] = true
		}
	}
	if err := s.available(ctx, b); err != nil {
		b.State, b.Error = "paused", err.Error()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return Batch{}, err
	}
	if err := s.noActive(ctx, 0); err != nil {
		return Batch{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, "INSERT INTO panda_deliveries (state, data) VALUES ($1, '{}') RETURNING id", b.State).Scan(&b.ID); err != nil {
		return Batch{}, err
	}
	if err := saveMetadata(ctx, tx, &b); err != nil {
		return Batch{}, err
	}
	if err := insertItems(ctx, tx, b); err != nil {
		return Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Batch{}, err
	}
	s.notify()
	return b, nil
}

// Stop finishes the currently running archive before leaving the remaining items.
func (s *Service) Stop(ctx context.Context, id int64) (Batch, error) {
	return s.change(ctx, id, func(b *Batch) error {
		if b.State != "running" && b.State != "paused" {
			return ErrInvalid
		}
		b.StopRequested = true
		if b.State == "paused" {
			b.State = "stopped"
		}
		return nil
	})
}

func (s *Service) RetryCleanup(ctx context.Context, id int64) (Batch, error) {
	return s.change(ctx, id, func(b *Batch) error {
		found := false
		for i := range b.Items {
			if b.Items[i].State == "cleanup_pending" {
				b.Items[i].CleanupAttempts = 0
				b.Items[i].checkpoint.NextCleanup = time.Time{}
				found = true
			}
		}
		if !found {
			return ErrInvalid
		}
		return nil
	})
}

func archiveName(id int64) string { return fmt.Sprintf("[%d].zip", id) }
func stagingPath(b Batch, item Item) string {
	return filepath.Join(b.root, ".tana-delivery", item.checkpoint.Token+".part")
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
		for s.ctx.Err() == nil {
			work, err := s.step()
			if err != nil {
				if s.ctx.Err() == nil {
					s.logger.Error("library_delivery_failed", "error", err)
				}
				break
			}
			if !work {
				break
			}
		}
	}
}

func (s *Service) step() (bool, error) {
	s.mu.Lock()
	b, result, err := s.claim()
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	switch result {
	case claimSettled:
		s.mu.Unlock()
		return true, nil
	case claimItem:
		s.mu.Unlock()
		err := s.deliver(b, 0)
		if errors.Is(err, errItemChanged) {
			// Another transaction moved the item on and keeps it current.
			return true, nil
		}
		if err != nil {
			return true, err
		}
		err = s.updateBatch(s.ctx, b.ID, func(current *Batch) {
			current.CurrentGalleryID = 0
		})
		return true, err
	}
	b, err = s.nextCleanup(s.ctx)
	s.mu.Unlock()
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	err = s.cleanup(b, 0)
	if errors.Is(err, errItemChanged) {
		err = nil
	}
	return true, err
}

type claimResult int

const (
	// claimNone: no running batch, or another transaction holds the batch or
	// its next item.
	claimNone claimResult = iota
	// claimSettled: the running batch finished or stopped.
	claimSettled
	// claimItem: the batch's Items holds the claimed item.
	claimItem
)

func (s *Service) claim() (Batch, claimResult, error) {
	tx, err := s.db.Begin(s.ctx)
	if err != nil {
		return Batch{}, claimNone, err
	}
	defer tx.Rollback(s.ctx)
	b, result, err := claimNext(s.ctx, tx)
	if err != nil || result == claimNone {
		return b, result, err
	}
	return b, result, tx.Commit(s.ctx)
}

// claimNext advances the running batch in tx. The batch row and then its next
// item are locked with SKIP LOCKED; the item is claimed by a transition from the
// state it was read in (a queued item starts transferring) and recorded as the
// batch's current item.
func claimNext(ctx context.Context, tx pgx.Tx) (Batch, claimResult, error) {
	var data []byte
	err := tx.QueryRow(ctx, "SELECT data FROM panda_deliveries WHERE state = 'running' FOR UPDATE SKIP LOCKED").Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, claimNone, nil
	}
	if err != nil {
		return Batch{}, claimNone, err
	}
	b, err := decode(data)
	if err != nil {
		return Batch{}, claimNone, err
	}
	result := claimSettled
	// Advance a whole archive before the next one, even after Stop is requested.
	if b.StopRequested && b.CurrentGalleryID == 0 {
		b.State = "stopped"
		err = finish(ctx, tx, &b)
	} else {
		var item Item
		item, err = lockNextItem(ctx, tx, b)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			err = finish(ctx, tx, &b)
		case err == nil:
			claimed := item
			if claimed.State == "queued" {
				claimed.State = "transferring"
			}
			err = saveItem(ctx, tx, b.ID, item.State, claimed)
			b.CurrentGalleryID = item.GalleryID
			// Transfers need only their own checkpoint, regardless of snapshot size.
			b.Items = []Item{claimed}
			result = claimItem
		}
	}
	if errors.Is(err, errItemChanged) {
		return Batch{}, claimNone, nil
	}
	if err != nil {
		return Batch{}, claimNone, err
	}
	if err := saveMetadata(ctx, tx, &b); err != nil {
		return Batch{}, claimNone, err
	}
	return b, result, nil
}
