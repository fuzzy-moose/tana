package status

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// The sources are interfaces because metadataproxy imports metadata, whose in-package tests
// construct status.Service, so status must not import metadataproxy directly.
// *downloads.Service, *sitemap.Service and *metadataproxy.Service satisfy them.
type DownloadSource interface {
	Storage() downloads.StorageStatus
}

type SitemapSource interface {
	Status(ctx context.Context) (collectorapi.SitemapStatus, error)
}

type ProxySource interface {
	Status(ctx context.Context) (collectorapi.MetadataProxyStatus, error)
}

// Summary aggregates the facts behind the collector sidebar labels and Overview attention list.
func (s *Service) Summary(ctx context.Context) (collectorapi.Summary, error) {
	var result collectorapi.Summary
	counts, err := s.q.DownloadSummaryCounts(ctx)
	if err != nil {
		return result, err
	}
	storage := collectorapi.DownloadStorage(s.downloads.Storage())
	result.Downloads = collectorapi.SummaryDownloads{Active: counts.Active, Failed: counts.Failed, Storage: &storage}
	if result.Downloads.Failed > 0 {
		if result.Downloads.LatestError, err = s.q.LatestDownloadFailure(ctx); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return result, err
		}
	}

	favorites, err := s.Favorites(ctx)
	if err != nil {
		return result, err
	}
	result.Favorites.AuthenticatedCooldownUntil = favorites.AuthenticatedCooldownUntil
	for _, category := range favorites.Categories {
		if category.State == "running" || category.State == "waiting_cooldown" || category.Queued {
			result.Favorites.Syncing = true
		}
	}

	retry, err := s.q.MetadataRetry(ctx)
	if err != nil {
		return result, err
	}
	result.Metadata.MainBackgroundPaused = retry.MainBackgroundPaused
	until, err := s.ban.Until(ctx)
	if err != nil {
		return result, err
	}
	if until.After(time.Now()) {
		result.Metadata.UpstreamCooldownUntil = &until
	}

	sitemap, err := s.sitemap.Status(ctx)
	if err != nil {
		return result, err
	}
	result.Sitemap = collectorapi.SummarySitemap{State: sitemap.State, ChildrenTotal: sitemap.ChildrenTotal,
		ChildrenProcessed: sitemap.ChildrenCompleted + sitemap.ChildrenSkipped + sitemap.ChildrenFailed}

	if result.Feeds.FailedCaptures, err = s.q.FailedCaptureCount(ctx); err != nil {
		return result, err
	}
	if result.Feeds.FailedCaptures > 0 {
		if result.Feeds.LatestError, err = s.q.LatestCaptureFailure(ctx); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return result, err
		}
	}
	if result.ReferenceImports.Active, err = s.q.ActiveReferenceImports(ctx); err != nil {
		return result, err
	}

	proxies, err := s.proxies.Status(ctx)
	if err != nil {
		return result, err
	}
	result.Proxies.Enabled = proxies.Enabled
	for _, channel := range proxies.Channels {
		if channel.BanUntil != nil {
			result.Proxies.ChannelsInCooldown++
		}
	}
	return result, nil
}
