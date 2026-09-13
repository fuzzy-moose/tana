// Package delivery durably transfers retained collector archives into libraries.
package delivery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

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
	db        *sql.DB
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
func New(ctx context.Context, db *sql.DB, client collector, libraries libraryCatalog, imports importer, logger *slog.Logger) (*Service, error) {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "INSERT INTO panda_deliveries (state, data) VALUES (?, '{}')", b.State)
	if err != nil {
		return Batch{}, err
	}
	b.ID, err = result.LastInsertId()
	if err != nil {
		return Batch{}, err
	}
	if err := saveMetadata(ctx, tx, &b); err != nil {
		return Batch{}, err
	}
	if err := insertItems(ctx, tx, b); err != nil {
		return Batch{}, err
	}
	if err := tx.Commit(); err != nil {
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
	var data []byte
	err := s.db.QueryRowContext(s.ctx, "SELECT data FROM panda_deliveries WHERE state = 'running'").Scan(&data)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.mu.Unlock()
		return false, err
	}
	if err == nil {
		b, err := decode(data)
		if err != nil {
			s.mu.Unlock()
			return false, err
		}
		// Advance a whole archive before the next one, even after Stop is requested.
		if b.StopRequested && b.CurrentGalleryID == 0 {
			b.State = "stopped"
			err := saveMetadata(s.ctx, s.db, &b)
			s.mu.Unlock()
			return true, err
		}
		item, err := s.nextItem(s.ctx, b)
		if errors.Is(err, sql.ErrNoRows) {
			if err := finish(s.ctx, s.db, &b); err != nil {
				s.mu.Unlock()
				return false, err
			}
			err := saveMetadata(s.ctx, s.db, &b)
			s.mu.Unlock()
			return true, err
		}
		if err != nil {
			s.mu.Unlock()
			return false, err
		}
		b.CurrentGalleryID = item.GalleryID
		if err := saveMetadata(s.ctx, s.db, &b); err != nil {
			s.mu.Unlock()
			return false, err
		}
		// Transfers need only their own checkpoint, regardless of snapshot size.
		b.Items = []Item{item}
		s.mu.Unlock()
		if err := s.deliver(b, 0); err != nil {
			return true, err
		}
		err = s.updateBatch(s.ctx, b.ID, func(current *Batch) {
			current.CurrentGalleryID = 0
		})
		return true, err
	}
	b, err := s.nextCleanup(s.ctx)
	s.mu.Unlock()
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, s.cleanup(b, 0)
}
