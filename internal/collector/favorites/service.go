// Package favorites collects Panda favorite categories only on explicit request.
package favorites

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrInvalidCategory = errors.New("favorite category must be 0–9")
var ErrClosed = errors.New("favorite collection is stopped")

type PageClient interface {
	GetFavoritesPage(context.Context, int, string) (panda.FavoritesPage, error)
}

type request struct {
	category int
	full     bool
}

type Service struct {
	store   *store
	client  PageClient
	logger  *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	wake    chan struct{}
	mu      sync.Mutex
	pending []request
	active  *request
	runtime [10]collectorapi.FavoriteCategory
}

func New(ctx context.Context, db *sql.DB, cfg panda.FavoritesConfig, client PageClient, logger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{
		store:  &store{db: db, q: dbgen.New(db), host: cfg.Origin(), accountKey: cfg.AccountKey},
		client: client, logger: logger, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1),
	}
	s.workers.Go(func() { s.run(ctx) })
	return s
}

func (s *Service) Close() { s.cancel(); s.workers.Wait() }

// Enqueue coalesces each category. A full re-sync upgrades queued work, or
// follows an incremental run already in progress. Request cancellation does
// not cancel accepted work; process shutdown does.
func (s *Service) Enqueue(category int, full bool) error {
	if category < 0 || category > 9 {
		return ErrInvalidCategory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return ErrClosed
	}
	s.enqueueLocked(category, full)
	return nil
}

// EnqueueAll admits all ten categories together, using the same coalescing as
// individual requests. No upstream calls happen during admission.
func (s *Service) EnqueueAll(full bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return ErrClosed
	}
	for category := range 10 {
		s.enqueueLocked(category, full)
	}
	return nil
}

func (s *Service) enqueueLocked(category int, full bool) {
	if s.active != nil && s.active.category == category && (!full || s.active.full) {
		return
	}
	for i := range s.pending {
		if s.pending[i].category == category {
			s.pending[i].full = s.pending[i].full || full
			return
		}
	}
	s.pending = append(s.pending, request{category: category, full: full})
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		s.mu.Lock()
		if len(s.pending) == 0 {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		}
		job := s.pending[0]
		s.pending = s.pending[1:]
		s.active = &job
		started := time.Now()
		s.runtime[job.category].StartedAt = &started
		s.mu.Unlock()
		for failures := int64(0); ctx.Err() == nil; failures++ {
			s.mu.Lock()
			s.runtime[job.category].RetryAt = nil
			s.mu.Unlock()
			err := s.collect(ctx, job)
			at := time.Now()
			if err == nil {
				s.mu.Lock()
				s.runtime[job.category].LastOutcome = "success"
				s.runtime[job.category].FinishedAt = &at
				s.mu.Unlock()
				s.logger.Info("favorites_sync_completed", "category", job.category, "full", job.full)
				break
			}
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("favorites_sync_failed", "category", job.category, "full", job.full, "error", err)
			s.mu.Lock()
			s.runtime[job.category].LastError = err.Error()
			s.runtime[job.category].LastErrorAt = &at
			if !retryable(err) {
				s.runtime[job.category].LastOutcome = "failed"
				s.runtime[job.category].FinishedAt = &at
				s.mu.Unlock()
				break
			}
			delay := panda.RetryDelay(failures, err, at)
			retryAt := at.Add(delay)
			s.runtime[job.category].RetryAt = &retryAt
			s.mu.Unlock()
			s.logger.Info("favorites_sync_retry", "category", job.category, "delay", delay)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		s.mu.Lock()
		s.active = nil
		s.mu.Unlock()
	}
}

func retryable(err error) bool {
	if errors.Is(err, panda.ErrFavoritesPage) {
		return false
	}
	if e, ok := errors.AsType[*panda.HTTPError](err); ok {
		return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
	}
	return true
}

func (s *Service) collect(ctx context.Context, job request) error {
	known, initialized, err := s.store.known(ctx, job.category)
	if err != nil {
		return err
	}
	full := job.full || !initialized
	var entries []panda.Favorite
	var name, next string
	var last time.Time
	visited := make(map[string]bool)
	seen := make(map[int64]bool)
	for {
		if visited[next] {
			return fmt.Errorf("%w: pagination loop", panda.ErrFavoritesPage)
		}
		visited[next] = true
		page, err := s.client.GetFavoritesPage(ctx, job.category, next)
		if err != nil {
			return err
		}
		name = page.CategoryName
		newEntries := 0
		for _, entry := range page.Entries {
			if seen[entry.GalleryRef.ID] || (!last.IsZero() && entry.AddedAt.After(last)) {
				return fmt.Errorf("%w: duplicate or out-of-order favorites across pages", panda.ErrFavoritesPage)
			}
			seen[entry.GalleryRef.ID], last = true, entry.AddedAt
			if at, ok := known[entry.GalleryRef.ID]; !ok || at != entry.AddedAt.Unix() {
				newEntries++
			}
			entries = append(entries, entry)
		}
		if page.Next == "" || (!full && newEntries < panda.FavoritesPageSize) {
			break
		}
		next = page.Next
	}
	return s.store.complete(ctx, job.category, name, full, entries)
}
