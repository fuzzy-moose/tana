package favorites

import (
	"context"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func (s *Service) Status(ctx context.Context) (collectorapi.FavoritesStatus, error) {
	rows, err := s.store.q.CategoryStatistics(ctx, dbgen.CategoryStatisticsParams{
		Host: s.store.host, AccountKey: s.store.accountKey,
	})
	if err != nil {
		return collectorapi.FavoritesStatus{}, err
	}
	s.mu.Lock()
	categories := append([]collectorapi.FavoriteCategory(nil), s.runtime[:]...)
	for category := range categories {
		categories[category].Category = category
		categories[category].State = "idle"
	}
	for _, job := range s.pending {
		categories[job.category].State = "queued"
		categories[job.category].Queued = true
		categories[job.category].QueuedFull = job.full
	}
	if job := s.active; job != nil {
		category := &categories[job.category]
		category.State = "running"
		category.Full = job.full
		if category.RetryAt != nil && category.RetryAt.After(time.Now()) {
			category.State = "waiting_cooldown"
		}
	}
	s.mu.Unlock()
	for _, row := range rows {
		category := &categories[row.Category]
		category.Name = row.Name
		category.Favorites = row.Favorites
		at := time.UnixMilli(row.SyncedAt)
		category.LastSyncedAt = &at
	}
	return collectorapi.FavoritesStatus{Host: s.store.host, AccountKey: s.store.accountKey, Categories: categories}, nil
}
