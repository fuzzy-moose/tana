// Package favorites collects explicitly requested Panda favorite categories,
// including unfinished requests retained across collector restarts.
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
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrInvalidCategory = errors.New("favorite category must be 0–9")
var ErrClosed = errors.New("favorite collection is stopped")

type PageClient interface {
	GetFavoritesPage(context.Context, int, string) (panda.FavoritesPage, error)
}

type Service struct {
	store   *store
	client  PageClient
	logger  *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	wake    chan struct{}
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

// Enqueue durably coalesces each category. A full re-sync upgrades queued work,
// or follows an incremental run in progress. Failed work resumes its checkpoint.
func (s *Service) Enqueue(category int, full bool) error {
	if category < 0 || category > 9 {
		return ErrInvalidCategory
	}
	return s.enqueue([]int{category}, full)
}

// EnqueueAll admits all ten categories in a single transaction.
func (s *Service) EnqueueAll(full bool) error {
	categories := make([]int, 10)
	for category := range categories {
		categories[category] = category
	}
	return s.enqueue(categories, full)
}

func (s *Service) enqueue(categories []int, full bool) error {
	if s.ctx.Err() != nil {
		return ErrClosed
	}
	if err := s.store.enqueue(s.ctx, categories, full); err != nil {
		return err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *Service) wait(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	case <-s.wake:
	}
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		current, err := s.store.next(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			s.wait(ctx, time.Second)
			continue
		}
		if err != nil {
			s.logger.Error("favorites_queue_failed", "error", err)
			s.wait(ctx, time.Second)
			continue
		}
		if delay := time.Until(time.UnixMilli(current.RetryAt)); delay > 0 {
			s.wait(ctx, delay)
			continue
		}
		if err := s.collect(ctx, current); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("favorites_sync_failed", "category", current.category, "error", err)
			if recordErr := s.store.failed(ctx, current, err); recordErr != nil {
				s.logger.Error("favorites_failure_record_failed", "error", recordErr)
				s.wait(ctx, time.Second)
			}
		}
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

// A rejected continuation gets one fresh traversal. Authentication, bans and
// transient failures retain the checkpoint; a broken profile must still fail.
func invalidContinuation(err error) bool {
	if errors.Is(err, panda.ErrFavoritesPage) {
		return true
	}
	if e, ok := errors.AsType[*panda.HTTPError](err); ok {
		switch e.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusGone, http.StatusUnprocessableEntity:
			return true
		}
	}
	return false
}

func (s *Service) collect(ctx context.Context, current job) error {
	err := s.collectPage(ctx, current)
	if err != nil && current.NextUrl != "" && current.Restarted == 0 && invalidContinuation(err) {
		if restartErr := s.store.restart(ctx, current.CategoryID); restartErr != nil {
			return restartErr
		}
		if s.logger != nil {
			s.logger.Info("favorites_traversal_restarted", "category", current.category, "error", err)
		}
		return nil
	}
	return err
}

func (s *Service) collectPage(ctx context.Context, current job) error {
	known, _, err := s.store.known(ctx, current.category)
	if err != nil {
		return err
	}
	visited, err := s.store.q.VisitedPages(ctx, current.CategoryID)
	if err != nil {
		return err
	}
	for _, url := range visited {
		if url == current.NextUrl {
			return fmt.Errorf("%w: pagination loop", panda.ErrFavoritesPage)
		}
	}
	seenIDs, err := s.store.q.SeenFavorites(ctx, current.CategoryID)
	if err != nil {
		return err
	}
	seen := make(map[int64]bool, len(seenIDs))
	for _, id := range seenIDs {
		seen[id] = true
	}
	page, err := s.client.GetFavoritesPage(ctx, current.category, current.NextUrl)
	if err != nil {
		return err
	}
	last := current.LastAddedAt
	reachedKnown := false
	for _, entry := range page.Entries {
		id, at := entry.GalleryRef.ID, entry.AddedAt.Unix()
		if !seen[id] {
			if last != 0 && at > last {
				return fmt.Errorf("%w: out-of-order favorites across pages", panda.ErrFavoritesPage)
			}
			last = at
		}
		seen[id] = true
		if previous, ok := known[id]; ok && previous == at {
			reachedKnown = true
		}
	}
	done := page.Next == "" || (current.Full == 0 && reachedKnown)
	if err := s.store.savePage(ctx, current, page, done, last); err != nil {
		return err
	}
	if s.logger != nil {
		event := "favorites_page_saved"
		if done {
			event = "favorites_sync_completed"
		}
		s.logger.Info(event, "category", current.category, "full", current.Full != 0, "entries_saved", len(seen), "pages_saved", current.PagesSaved+1)
	}
	return nil
}
