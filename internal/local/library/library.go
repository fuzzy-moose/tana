// Package library owns registered library roots, persistence, and availability.
package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/library/dbgen"
)

const (
	checkInterval = 60 * time.Second
	checkTimeout  = 5 * time.Second
	checkWorkers  = 4
)

var (
	ErrInvalidName     = errors.New("name must not be empty")
	ErrInvalidPath     = errors.New("path must be absolute")
	ErrRootUnavailable = errors.New("root must exist and be a directory")
	ErrRootConflict    = errors.New("root overlaps an existing library")
	ErrStorageOverlap  = errors.New("root contains application storage")
	ErrNotFound        = errors.New("library not found")
)

type Library struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Path          string     `json:"path"`
	Availability  string     `json:"availability"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
}

type Service struct {
	db      *sql.DB
	queries *dbgen.Queries
	dataDir string
	probe   *filesystemProbe
	logger  *slog.Logger
	timeout time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	queue  []string
	// Includes queued and running checks so repeated refreshes coalesce.
	pending map[string]bool
	wake    chan struct{}
}

// Open migrates local storage and starts availability checks. Close after HTTP
// requests have drained. The supplied context controls the background workers.
func Open(ctx context.Context, dataDir string, logger *slog.Logger) (*Service, error) {
	s, err := openService(ctx, dataDir, logger)
	if err != nil {
		return nil, err
	}
	if err := s.queries.ResetAvailability(ctx); err != nil {
		_ = s.db.Close()
		return nil, err
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.start(ctx, checkInterval)
	return s, nil
}

func openService(ctx context.Context, dir string, logger *slog.Logger) (*Service, error) {
	db, dir, err := openDatabase(ctx, dir)
	if err != nil {
		return nil, err
	}
	s := &Service{
		db: db, queries: dbgen.New(db), dataDir: dir, probe: newFilesystemProbe(),
		logger: logger, timeout: checkTimeout, pending: make(map[string]bool), wake: make(chan struct{}, checkWorkers),
	}
	roots, err := s.queries.ListLibraries(ctx)
	if err == nil {
		for _, root := range roots {
			if containsPath(root.Path, dir) {
				err = ErrStorageOverlap
				break
			}
		}
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Service) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	return s.db.Close()
}

func (s *Service) Create(ctx context.Context, name, path string) (Library, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	name = strings.TrimSpace(name)
	if name == "" {
		return Library{}, ErrInvalidName
	}
	if !filepath.IsAbs(path) {
		return Library{}, ErrInvalidPath
	}
	// Do not clean away '..' before resolving symlinks: that can change which
	// directory the original filesystem path names.
	root, err := s.probe.check(ctx, path, true)
	if err != nil {
		if ctx.Err() != nil {
			return Library{}, ctx.Err()
		}
		return Library{}, fmt.Errorf("%w: %v", ErrRootUnavailable, err)
	}
	if containsPath(root, s.dataDir) {
		return Library{}, ErrStorageOverlap
	}
	// An immediate SQLite transaction serializes validation and insertion,
	// including registrations through another connection to the same database.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Library{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	roots, err := q.ListLibraries(ctx)
	if err != nil {
		return Library{}, err
	}
	for _, existing := range roots {
		if containsPath(root, existing.Path) || containsPath(existing.Path, root) {
			return Library{}, ErrRootConflict
		}
	}
	row, err := q.CreateLibrary(ctx, dbgen.CreateLibraryParams{
		ID: rand.Text(), Name: name, Path: root,
		LastCheckedAt: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
	})
	if err != nil {
		return Library{}, err
	}
	if err := tx.Commit(); err != nil {
		return Library{}, err
	}
	return fromRow(row), nil
}

func (s *Service) List(ctx context.Context) ([]Library, error) {
	rows, err := s.queries.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Library, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRow(row))
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, id string) (Library, error) {
	row, err := s.queries.GetLibrary(ctx, id)
	return fromRow(row), domainError(err)
}

func (s *Service) Rename(ctx context.Context, id, name string) (Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Library{}, ErrInvalidName
	}
	row, err := s.queries.RenameLibrary(ctx, dbgen.RenameLibraryParams{ID: id, Name: name})
	return fromRow(row), domainError(err)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	// The database owns library records; this operation never visits the root.
	return s.queries.DeleteLibrary(ctx, id)
}

func (s *Service) RequestCheck(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	s.enqueue(id)
	return nil
}

func containsPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func fromRow(row dbgen.Library) Library {
	library := Library{ID: row.ID, Name: row.Name, Path: row.Path, Availability: row.Availability}
	if row.LastCheckedAt.Valid {
		t := time.UnixMilli(row.LastCheckedAt.Int64).UTC()
		library.LastCheckedAt = &t
	}
	return library
}

func domainError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) start(ctx context.Context, interval time.Duration) {
	for range checkWorkers {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-s.wake:
				}
				for ctx.Err() == nil {
					s.mu.Lock()
					if len(s.queue) == 0 {
						s.mu.Unlock()
						break
					}
					id := s.queue[0]
					s.queue[0] = ""
					s.queue = s.queue[1:]
					s.mu.Unlock()
					s.check(ctx, id)
					s.mu.Lock()
					delete(s.pending, id)
					s.mu.Unlock()
				}
			}
		}()
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			roots, err := s.queries.ListLibraries(ctx)
			if err != nil && ctx.Err() == nil {
				s.logger.Error("library_checks_failed", "error", err)
			}
			for _, root := range roots {
				s.enqueue(root.ID)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) enqueue(id string) {
	s.mu.Lock()
	if !s.pending[id] {
		s.pending[id] = true
		s.queue = append(s.queue, id)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) check(ctx context.Context, id string) {
	root, err := s.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, ErrNotFound) && ctx.Err() == nil {
			s.logger.Error("library_check_failed", "library_id", id, "error", err)
		}
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, s.timeout)
	_, err = s.probe.check(probeCtx, root.Path, false)
	cancel()
	if ctx.Err() != nil {
		return // Shutdown is not evidence of unavailability.
	}
	availability := "available"
	if err != nil {
		availability = "unavailable"
	}
	// A late filesystem result cannot overwrite this observation: the probe
	// itself never writes to storage. A deleted library is not re-created.
	err = s.queries.UpdateAvailability(ctx, dbgen.UpdateAvailabilityParams{
		ID: id, Availability: availability,
		LastCheckedAt: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
	})
	if err != nil && ctx.Err() == nil {
		s.logger.Error("library_check_failed", "library_id", id, "error", err)
	}
}
