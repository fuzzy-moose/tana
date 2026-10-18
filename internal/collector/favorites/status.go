package favorites

import (
	"context"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func (s *Service) Status(ctx context.Context) (collectorapi.FavoritesStatus, error) {
	downloads := collectorapi.FavoriteDownloadsStatus{FavoriteDownloadSettings: collectorapi.FavoriteDownloadSettings{Categories: []int{}}}
	categories := make([]collectorapi.FavoriteCategory, 10)
	for i := range categories {
		categories[i].Category, categories[i].State = i, "idle"
	}
	err := s.store.transaction(ctx, func(q *dbgen.Queries) error {
		state, err := q.DownloadBaselineState(ctx)
		if err != nil {
			return err
		}
		downloads.BaselineState = state
		completed, err := q.BaselineCategories(ctx)
		if err != nil {
			return err
		}
		downloads.BaselineCategories = len(completed)
		enabled, err := q.DownloadCategories(ctx)
		if err != nil {
			return err
		}
		for _, category := range enabled {
			downloads.Categories = append(downloads.Categories, int(category))
		}
		rows, err := q.CategoryStatistics(ctx, dbgen.CategoryStatisticsParams{Host: s.store.host, AccountKey: s.store.accountKey})
		if err != nil {
			return err
		}
		for _, row := range rows {
			category := &categories[row.Category]
			category.Name, category.Favorites = row.Name, row.Favorites
			category.LastSyncedAt = row.SyncedAt
		}
		syncs, err := q.ListSyncs(ctx, dbgen.ListSyncsParams{Host: s.store.host, AccountKey: s.store.accountKey})
		if err != nil {
			return err
		}
		for _, row := range syncs {
			current := row.FavoriteSync
			category := &categories[row.Category]
			category.State = current.State
			if current.State == "success" || current.State == "failed" {
				category.State, category.LastOutcome = "idle", current.State
			}
			category.Full = current.Full
			category.Queued = current.State == "queued" || current.FollowupFull
			category.QueuedFull = current.FollowupFull || (current.State == "queued" && current.Full)
			category.StartedAt, category.FinishedAt = current.StartedAt, current.FinishedAt
			category.LastError, category.LastErrorAt = current.LastError, current.LastErrorAt
			category.RetryAt = current.RetryAt
			category.PagesSaved, category.EntriesSaved = current.PagesSaved, current.EntriesSaved
			category.LastSavedAt = current.LastSavedAt
			if current.State == "running" && current.RetryAt != nil && current.RetryAt.After(time.Now()) {
				category.State = "waiting_cooldown"
			}
		}
		return nil
	})
	if err != nil {
		return collectorapi.FavoritesStatus{}, err
	}
	return collectorapi.FavoritesStatus{Host: s.store.host, AccountKey: s.store.accountKey, Categories: categories, Downloads: downloads}, nil
}
