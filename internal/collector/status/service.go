// Package status assembles a read-only view of collector inventory and work.
package status

import (
	"context"
	"database/sql"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/status/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

type Service struct {
	q         *dbgen.Queries
	favorites *favorites.Service
	ban       *pandaban.State
}

func New(db *sql.DB, favorites *favorites.Service, ban *pandaban.State) *Service {
	return &Service{q: dbgen.New(db), favorites: favorites, ban: ban}
}

func (s *Service) Get(ctx context.Context) (collectorapi.Status, error) {
	result := collectorapi.Status{MetadataErrors: []collectorapi.MetadataError{}}
	favorites, err := s.favorites.Status(ctx)
	if err != nil {
		return result, err
	}
	result.Favorites = favorites
	counts, err := s.q.InventoryStatistics(ctx)
	if err != nil {
		return result, err
	}
	result.Inventory = collectorapi.InventoryStatus{
		GalleryReferences: counts.GalleryReferences, MetadataAvailable: counts.MetadataAvailable,
		MetadataPending: counts.MetadataPending, MetadataFailed: counts.MetadataFailed,
		FetchesPending: counts.FetchesPending, FetchesFailed: counts.FetchesFailed,
	}
	retry, err := s.q.MetadataRetry(ctx)
	if err != nil {
		return result, err
	}
	result.MetadataLastError = retry.LastError.String
	if at := time.UnixMilli(retry.NextAttemptAt); at.After(time.Now()) {
		result.MetadataRetryAt = &at
	}
	rows, err := s.q.RecentMetadataErrors(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		item := collectorapi.MetadataError{GalleryID: row.GalleryID, Error: row.MetadataError.String}
		if row.MetadataAttemptedAt.Valid {
			at := time.UnixMilli(row.MetadataAttemptedAt.Int64)
			item.At = &at
		}
		result.MetadataErrors = append(result.MetadataErrors, item)
	}
	until, err := s.ban.Until(ctx)
	if err != nil {
		return result, err
	}
	if until.After(time.Now()) {
		result.UpstreamCooldownUntil = &until
		for i := range result.Favorites.Categories {
			category := &result.Favorites.Categories[i]
			if category.State == "running" || category.State == "waiting_cooldown" {
				category.State = "waiting_cooldown"
				if category.RetryAt == nil || category.RetryAt.Before(until) {
					category.RetryAt = &until
				}
			}
		}
	}
	return result, nil
}
