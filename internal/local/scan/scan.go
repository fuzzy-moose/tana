// Package scan discovers new library sources and imports their inventories and galleries.
package scan

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/metadata/galleryinfo"
	"github.com/fuzzy-moose/tana/internal/local/source"
)

const importWorkers = 4

var ErrActive = errors.New("a scan is already active")

type Status struct {
	Phase            string     `json:"phase"`
	LibraryID        string     `json:"library_id,omitempty"`
	LibrariesTotal   int        `json:"libraries_total"`
	Discovered       int        `json:"discovered"`
	Imported         int        `json:"imported"`
	FailedSources    int        `json:"failed_sources"`
	Skipped          int        `json:"skipped"`
	DiscoveryErrors  int        `json:"discovery_errors"`
	GalleriesCreated int        `json:"galleries_created"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
}

type libraryCatalog interface {
	List(context.Context) ([]library.Library, error)
	Get(context.Context, string) (library.Library, error)
}

type Service struct {
	db        *sql.DB
	libraries libraryCatalog
	sources   *source.SQLiteRepository
	galleries *gallery.SQLiteRepository
	provider  metadata.Provider
	dirFS     func(string) fs.FS
	logger    *slog.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	status    Status
}

// New owns at most one scan until Close. dirFS supplies library filesystems
// (os.DirFS in production); archive files must implement io.ReaderAt.
func New(ctx context.Context, db *sql.DB, libraries libraryCatalog, dirFS func(string) fs.FS, logger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(ctx)
	return &Service{
		db: db, libraries: libraries, sources: source.NewSQLiteRepository(db),
		galleries: gallery.NewSQLiteRepository(db), provider: galleryinfo.Provider{}, dirFS: dirFS, logger: logger,
		ctx: ctx, cancel: cancel, status: Status{Phase: "idle"},
	}
}

// Request snapshots the selected libraries and starts work independently of the
// request context. An empty libraryID selects all registered libraries.
func (s *Service) Request(ctx context.Context, libraryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.status.Phase == "discovering" || s.status.Phase == "importing" {
		return ErrActive
	}
	var libraries []library.Library
	if libraryID == "" {
		var err error
		libraries, err = s.libraries.List(ctx)
		if err != nil {
			return err
		}
	} else {
		l, err := s.libraries.Get(ctx, libraryID)
		if err != nil {
			return err
		}
		libraries = []library.Library{l}
	}
	now := time.Now().UTC()
	s.status = Status{Phase: "discovering", LibraryID: libraryID, LibrariesTotal: len(libraries), StartedAt: &now}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run(libraries)
	}()
	return nil
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Close cancels work and waits for workers before the caller closes storage.
func (s *Service) Close() {
	s.mu.Lock()
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) run(libraries []library.Library) {
	candidates, err := s.discover(s.ctx, libraries)
	if err == nil {
		s.mu.Lock()
		s.status.Phase = "importing"
		s.mu.Unlock()
		err = s.importCandidates(s.ctx, candidates)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.status.FinishedAt = &now
	s.status.Phase = "completed"
	if s.status.FailedSources > 0 || s.status.DiscoveryErrors > 0 {
		s.status.Phase = "completed_with_errors"
	}
	if err != nil {
		s.status.Phase = "failed"
		s.status.Skipped = s.status.Discovered - s.status.Imported - s.status.FailedSources
		s.logger.Error("scan_failed", "error", err)
	}
}

type preparedSource struct {
	candidate candidate
	files     []string
	metadata  metadata.Values
	err       error
}

func (s *Service) importCandidates(ctx context.Context, candidates []candidate) error {
	results := make(chan preparedSource, importWorkers)
	var workers sync.WaitGroup
	for worker := range min(importWorkers, len(candidates)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := worker; i < len(candidates); i += importWorkers {
				if ctx.Err() != nil {
					return
				}
				c := candidates[i]
				result := s.prepareSource(ctx, c)
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()
	for result := range results {
		if ctx.Err() != nil {
			continue // Drain workers before releasing the scan slot or storage.
		}
		createdGallery := false
		err := result.err
		if err == nil {
			createdGallery, err = s.importSource(ctx, result.candidate, result.files, result.metadata)
		}
		s.mu.Lock()
		if err != nil {
			s.status.FailedSources++
		} else {
			s.status.Imported++
			if createdGallery {
				s.status.GalleriesCreated++
			}
		}
		s.mu.Unlock()
		if err != nil {
			s.logger.Error("source_import_failed", "library_id", result.candidate.libraryID, "path", result.candidate.path, "error", err)
		}
	}
	return ctx.Err()
}

func (s *Service) importSource(ctx context.Context, c candidate, files []string, values metadata.Values) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	imported, err := s.sources.CreateTx(ctx, tx, c.libraryID, c.path, c.kind, files)
	if err != nil {
		return false, err
	}
	_, err = s.galleries.CreateFromSourceTx(ctx, tx, imported.ID, values)
	createdGallery := err == nil
	if err != nil && !errors.Is(err, gallery.ErrNoImages) {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return createdGallery, nil
}
