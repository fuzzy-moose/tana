// Package collector assembles the metadata collector and owns its resources.
package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/sitemap"
	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type App struct {
	Logger           *slog.Logger
	Metadata         *metadata.Service
	Favorites        *favorites.Service
	Downloads        *downloads.Service
	Sitemap          *sitemap.Service
	ReferenceImports *metadata.ReferenceImports
	Status           *status.Service
	APIToken         string

	db    *sql.DB
	feeds *feed.Service
}

func New(ctx context.Context, cfg Config, logger *slog.Logger) (*App, error) {
	// Collection uses sustained pacing even if other Panda consumers allow bursts.
	limiter, err := panda.NewRateLimiter(cfg.Panda.RateInterval, 1)
	if err != nil {
		return nil, err
	}
	db, _, err := storage.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open collector storage: %w", err)
	}
	ban := pandaban.New(db)
	client, err := panda.NewClient(cfg.Panda.APIURL, &http.Client{
		Timeout: time.Minute, Transport: panda.RateLimitedTransport(limiter, panda.BanTransport(ban, nil)),
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	authLimiter, err := panda.NewRateLimiter(panda.AuthenticatedRateInterval, 1)
	if err != nil {
		db.Close()
		return nil, err
	}
	authClient, err := panda.NewAuthenticatedClient(cfg.AuthenticatedPanda, &http.Client{
		Timeout: time.Minute, Transport: panda.RateLimitedTransport(authLimiter, panda.BanTransport(ban, nil)),
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	downloadService, err := downloads.New(ctx, db, cfg.DownloadDir, authClient,
		downloads.NewHTTPTransfer(&http.Client{Timeout: 30 * time.Minute, Transport: panda.BanTransport(ban, nil)}), logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	favoritesService := favorites.New(ctx, db, cfg.AuthenticatedPanda, authClient, logger)
	sitemapLimiter, err := panda.NewRateLimiter(10*time.Second, 1)
	if err != nil {
		favoritesService.Close()
		downloadService.Close()
		db.Close()
		return nil, err
	}
	sitemapService, err := sitemap.New(ctx, db, cfg.Sitemap, &http.Client{
		// Redirects count as requests too; index children currently redirect hosts.
		Timeout: 5 * time.Minute, Transport: panda.RateLimitedTransport(sitemapLimiter, panda.BanTransport(ban, nil)),
	}, logger)
	if err != nil {
		favoritesService.Close()
		downloadService.Close()
		db.Close()
		return nil, err
	}
	imports, err := metadata.NewReferenceImports(ctx, db, filepath.Join(cfg.DataDir, "reference-imports"), logger)
	if err != nil {
		sitemapService.Close()
		favoritesService.Close()
		downloadService.Close()
		db.Close()
		return nil, err
	}
	return &App{
		Logger:           logger,
		APIToken:         cfg.APIToken,
		db:               db,
		feeds:            feed.New(ctx, db, cfg.Feed, nil, logger),
		Metadata:         metadata.New(ctx, db, client, logger),
		Favorites:        favoritesService,
		Downloads:        downloadService,
		Sitemap:          sitemapService,
		ReferenceImports: imports,
		Status:           status.New(db, favoritesService, ban, sitemapService),
	}, nil
}

// Close stops background work before releasing its database.
func (a *App) Close() error {
	a.ReferenceImports.Close()
	a.Sitemap.Close()
	a.Downloads.Close()
	a.Favorites.Close()
	a.feeds.Close()
	a.Metadata.Close()
	return a.db.Close()
}
