// Package library owns registered library roots, persistence, and availability.
package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	repository Repository
	dataDir    string
	probe      *filesystemProbe
	logger     *slog.Logger
	timeout    time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	queue  []string
	// Includes queued and running checks so repeated refreshes coalesce.
	pending map[string]bool
	wake    chan struct{}
}

// New starts availability checks using the supplied repository.
// dataDir must be the clean absolute application storage path, excluded from library
// roots. dirFS supplies a filesystem rooted at each native path (os.DirFS in
// production). Close after HTTP requests drain and before closing the database.
func New(ctx context.Context, repository Repository, dirFS func(string) fs.FS, dataDir string, logger *slog.Logger) (*Service, error) {
	s, err := newService(ctx, repository, dirFS, dataDir, logger)
	if err != nil {
		return nil, err
	}
	if err := s.repository.ResetAvailability(ctx); err != nil {
		return nil, err
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.start(ctx, checkInterval)
	return s, nil
}

func newService(ctx context.Context, repository Repository, dirFS func(string) fs.FS, dir string, logger *slog.Logger) (*Service, error) {
	s := &Service{
		repository: repository, dataDir: dir, probe: newFilesystemProbe(dirFS),
		logger: logger, timeout: checkTimeout, pending: make(map[string]bool), wake: make(chan struct{}, checkWorkers),
	}
	roots, err := s.repository.List(ctx)
	if err == nil {
		for _, root := range roots {
			if containsPath(root.Path, dir) {
				err = ErrStorageOverlap
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Close stops availability checks without closing the caller's database.
func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
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
	root := filepath.Clean(path)
	if err := s.probe.check(ctx, root); err != nil {
		if ctx.Err() != nil {
			return Library{}, ctx.Err()
		}
		return Library{}, fmt.Errorf("%w: %v", ErrRootUnavailable, err)
	}
	if containsPath(root, s.dataDir) {
		return Library{}, ErrStorageOverlap
	}
	return s.repository.Create(ctx, name, root)
}

func (s *Service) List(ctx context.Context) ([]Library, error) {
	return s.repository.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (Library, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) Rename(ctx context.Context, id, name string) (Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Library{}, ErrInvalidName
	}
	return s.repository.Rename(ctx, id, name)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repository.Delete(ctx, id)
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
			roots, err := s.repository.List(ctx)
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
	err = s.probe.check(probeCtx, root.Path)
	cancel()
	if ctx.Err() != nil {
		return // Shutdown is not evidence of unavailability.
	}
	if errors.Is(err, errProbeNotAdmitted) {
		// Waiting for probe capacity is not an observation of this root.
		s.logger.Warn("library_check_deferred", "library_id", id, "reason", "probe_not_admitted")
		return
	}
	availability := "available"
	if err != nil {
		availability = "unavailable"
	}
	// A late filesystem result cannot overwrite this observation: the probe
	// itself never writes to storage. A deleted library is not re-created.
	err = s.repository.UpdateAvailability(ctx, id, availability, time.Now())
	if err != nil && ctx.Err() == nil {
		s.logger.Error("library_check_failed", "library_id", id, "error", err)
	}
}
