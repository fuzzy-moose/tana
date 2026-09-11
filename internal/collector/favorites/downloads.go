package favorites

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// enqueue bootstraps every category even when the caller selects just one.
// Previously successful syncs are not proof of a complete download baseline.
func (s *store) enqueue(ctx context.Context, categories []int, full bool) error {
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		state, err := q.DownloadBaselineState(ctx)
		if err != nil {
			return err
		}
		if state == "ready" {
			return s.enqueueCategories(ctx, q, categories, full)
		}
		if state == "not_started" {
			if err := q.SeedFavoriteObservations(ctx); err != nil {
				return err
			}
			if err := q.StartDownloadBaseline(ctx); err != nil {
				return err
			}
		}
		completed, err := q.BaselineCategories(ctx)
		if err != nil {
			return err
		}
		for category := range 10 {
			if slices.Contains(completed, int64(category)) {
				continue
			}
			if state == "collecting" {
				if err := s.enqueueCategories(ctx, q, []int{category}, true); err != nil {
					return err
				}
				continue
			}
			id, err := q.SaveCategory(ctx, dbgen.SaveCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
			if err != nil {
				return err
			}
			current, err := q.GetSync(ctx, id)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if current.State == "running" {
				// Let an in-flight page commit, then refresh from the beginning.
				if err := q.QueueFullSync(ctx, id); err != nil {
					return err
				}
			} else if err := newSync(ctx, q, id, true); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) DownloadSettings(ctx context.Context) (collectorapi.FavoriteDownloadSettings, error) {
	rows, err := s.store.q.DownloadCategories(ctx)
	settings := collectorapi.FavoriteDownloadSettings{Categories: make([]int, 0, len(rows))}
	for _, category := range rows {
		settings.Categories = append(settings.Categories, int(category))
	}
	return settings, err
}

func (s *Service) SetDownloadSettings(ctx context.Context, settings collectorapi.FavoriteDownloadSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	return s.store.transaction(ctx, func(q *dbgen.Queries) error {
		if err := q.ClearDownloadCategories(ctx); err != nil {
			return err
		}
		for _, category := range settings.Categories {
			if err := q.EnableDownloadCategory(ctx, int64(category)); err != nil {
				return err
			}
		}
		return nil
	})
}
