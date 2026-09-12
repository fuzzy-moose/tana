package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/sitemap"
	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestSitemapControlsThroughLocalProxy(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Test durable API admission independently of upstream requests.
	logger := slog.New(slog.DiscardHandler)
	sitemaps, err := sitemap.New(t.Context(), db, sitemap.Config{URL: "https://panda.test/custom/index.xml?version=2"}, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	sitemaps.Close() // Stop the idle worker before submitting durable work.
	favs := favorites.New(ctx, db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "42"}, blockedFavorites{}, logger)
	defer favs.Close()
	ban := pandaban.New(db)
	until := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	if err := ban.Extend(t.Context(), until); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{
		Logger: logger, Sitemap: sitemaps, Favorites: favs, Ban: ban,
		Status: status.New(db, favs, ban), APIToken: "server-secret",
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: logger, Collector: client})
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		action, body string
		code         int
		state        string
	}{
		{"retry", `{}`, 409, ""},
		{"start", `{"force":"yes"}`, 400, ""},
		{"start", `{"force":true,"unknown":true}`, 400, ""},
		{"start", `{"force":true}`, 202, "running"},
		{"start", `{"force":false}`, 202, "running"},
		{"cancel", `{"force":true}`, 400, ""},
		{"cancel", `{}`, 202, "cancelled"},
		{"retry", `{}`, 202, "running"},
		{"unknown", `{}`, 404, ""},
	} {
		w := request("POST", "/api/collector/sitemap/"+tc.action, tc.body)
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d %s", tc.action, tc.body, w.Code, w.Body)
		}
		if tc.state != "" {
			var state collectorapi.SitemapStatus
			if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || state.State != tc.state || !state.Force {
				t.Fatalf("%s: %s, %v", tc.action, w.Body, err)
			}
			if state.State == "running" && (state.RetryAt == nil || !state.RetryAt.Equal(until)) {
				t.Fatalf("%s missing shared cooldown: %s", tc.action, w.Body)
			}
		}
	}
	w := request("GET", "/api/collector/sitemap/status", "")
	var sitemapStatus collectorapi.SitemapStatus
	if err := json.Unmarshal(w.Body.Bytes(), &sitemapStatus); err != nil || w.Code != 200 || sitemapStatus.State != "running" ||
		sitemapStatus.RetryAt == nil || !sitemapStatus.RetryAt.Equal(until) {
		t.Fatalf("sitemap status: %d %s, %v", w.Code, w.Body, err)
	}
	w = request("GET", "/api/collector/status", "")
	var connection collectorapi.ConnectionStatus
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || w.Code != 200 || connection.Status == nil ||
		!connection.Status.Available || strings.Contains(w.Body.String(), "server-secret") {
		t.Fatalf("collector status: %d %s, %v", w.Code, w.Body, err)
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-token")
	if err != nil {
		t.Fatal(err)
	}
	handler = NewHandler(&local.App{Logger: logger, Collector: wrong})
	w = request("POST", "/api/collector/sitemap/start", `{}`)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong credentials: %d %s", w.Code, w.Body)
	}
	handler = NewHandler(&local.App{Logger: logger})
	w = request("POST", "/api/collector/sitemap/start", `{}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing collector: %d %s", w.Code, w.Body)
	}
}
