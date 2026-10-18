package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/metadataproxy"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/sitemap"
	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestSummaryAggregatesCollectorState(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	logger := slog.New(slog.DiscardHandler)
	downloadService, err := downloads.New(t.Context(), db, t.TempDir(), waitingArchiveClient{}, nil, logger,
		downloads.StorageConfig{PauseBelowBytes: 1, ResumeAtBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer downloadService.Close()
	favoriteService := favorites.New(t.Context(), db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "current"}, nil, logger)
	defer favoriteService.Close()
	sitemapService, err := sitemap.New(t.Context(), db, sitemap.Config{URL: "https://panda.test/index.xml"}, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer sitemapService.Close()
	metadataService := metadata.New(t.Context(), db, nil, logger)
	defer metadataService.Close()
	proxies, err := metadataproxy.New(t.Context(), db, metadataService, panda.Config{APIURL: "https://panda.invalid/api", RateInterval: time.Second}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer proxies.Close()
	ban := pandaban.New(db)
	app := &collector.App{Logger: logger, APIToken: "test-token", Status: status.New(db, favoriteService, ban, pandaban.NewAuthenticated(db), downloadService, sitemapService, proxies)}
	handler := NewHandler(app)

	read := func() collectorapi.Summary {
		t.Helper()
		w := apiRequest(handler, "GET", "/api/summary", "")
		if w.Code != http.StatusOK {
			t.Fatalf("summary: %d %s", w.Code, w.Body)
		}
		var summary collectorapi.Summary
		if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
			t.Fatal(err)
		}
		return summary
	}
	empty := read()
	if empty.Downloads.Active != 0 || empty.Downloads.Failed != 0 || empty.Downloads.Storage == nil || empty.Favorites.Syncing ||
		empty.Feeds.FailedCaptures != 0 || empty.ReferenceImports.Active != 0 || empty.Proxies.ChannelsInCooldown != 0 || empty.Sitemap.State == "" {
		t.Fatalf("empty summary: %+v", empty)
	}

	now := time.Now()
	future := now.Add(time.Hour)
	if _, err := db.Exec(t.Context(), `
INSERT INTO panda_downloads (gallery_id, token, state, created_at, updated_at, last_error) VALUES
  (1, 'one', 'failed', to_timestamp(1), to_timestamp(1), 'old failure'), (2, 'two', 'failed', to_timestamp(1), to_timestamp(2), 'archive preparation timed out'),
  (3, 'three', 'queued', to_timestamp(3), to_timestamp(3), '');
INSERT INTO raw_feeds (id, captured_at, feed_url, body, last_error) VALUES
  (1, to_timestamp(1), 'https://panda.test/feed', '\x00', 'XML syntax error on line 12'), (2, to_timestamp(2), 'https://panda.test/feed', '\x00', NULL);
INSERT INTO reference_imports (id, filename, status, created_at, size_bytes) VALUES
  ('a', 'a.txt', 'processing', to_timestamp(1), 1), ('b', 'b.txt', 'completed', to_timestamp(1), 1);
INSERT INTO metadata_proxy_channels (id, name, endpoint, username, password, user_agent, enabled, ban_until, created_at) VALUES
  ('x', 'X', 'http://x.test:1', '', '', '', true, $1, to_timestamp(1)), ('y', 'Y', 'http://y.test:1', '', '', '', true, NULL, to_timestamp(1));
UPDATE metadata_proxy_settings SET enabled = true;
INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES (1, 'https://panda.test', 'current', 2, 'Manga', to_timestamp(0.001));
INSERT INTO favorite_syncs (category_id, state, "full", queued_at, retry_at) VALUES (1, 'running', false, to_timestamp(1), $2);
`, pgx.QueryExecModeSimpleProtocol, future, future); err != nil {
		t.Fatal(err)
	}
	if err := ban.Extend(t.Context(), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	summary := read()
	if !summary.Favorites.Syncing || summary.Downloads.Active != 1 || summary.Downloads.Failed != 2 || summary.Downloads.LatestError != "archive preparation timed out" ||
		summary.Feeds.FailedCaptures != 1 || summary.Feeds.LatestError != "XML syntax error on line 12" ||
		summary.ReferenceImports.Active != 1 || !summary.Proxies.Enabled || summary.Proxies.ChannelsInCooldown != 1 ||
		summary.Metadata.UpstreamCooldownUntil == nil {
		t.Fatalf("summary: %+v", summary)
	}
}
