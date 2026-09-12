package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type blockedFavorites struct{}

func (blockedFavorites) GetFavoritesPage(ctx context.Context, _ int, _ string) (panda.FavoritesPage, error) {
	<-ctx.Done()
	return panda.FavoritesPage{}, ctx.Err()
}

func TestCollectorConnectionDoesNotDependOnStatistics(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	logger := slog.New(slog.DiscardHandler)
	favs := favorites.New(ctx, db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "42"}, nil, logger)
	defer favs.Close()
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{
		Logger: logger, Favorites: favs, Status: status.New(db, favs, pandaban.New(db)), APIToken: "server-secret",
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: logger, Collector: client})
	// Reserving the sole connection makes any statistics read wait for release.
	busy, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	requestContext, requestCancel := context.WithTimeout(t.Context(), time.Second)
	defer requestCancel()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequestWithContext(requestContext, "GET", "/api/collector/status", nil))
	var connection collectorapi.ConnectionStatus
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || w.Code != 200 ||
		!connection.Reachable || connection.Authenticated == nil || !*connection.Authenticated || connection.Error != "" {
		t.Fatalf("busy database broke connection status: %d %s, %v", w.Code, w.Body, err)
	}
	if err := busy.Close(); err != nil {
		t.Fatal(err)
	}
	// An unavailable favorites view must not affect the inventory view.
	if _, err := db.Exec(`ALTER TABLE favorite_categories RENAME TO unavailable_favorite_categories`); err != nil {
		t.Fatal(err)
	}
	for _, view := range []struct {
		path string
		code int
	}{
		{"favorites", 502}, {"inventory", 200}, {"metadata", 200},
	} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/collector/"+view.path+"/status", nil))
		if w.Code != view.code {
			t.Fatalf("%s status: %d %s", view.path, w.Code, w.Body)
		}
	}
	if _, err := db.Exec(`ALTER TABLE gallery_refs RENAME TO unavailable_gallery_refs`); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"inventory", "metadata"} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/collector/"+view+"/status", nil))
		if w.Code != 502 {
			t.Fatalf("%s unavailable: %d %s", view, w.Code, w.Body)
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/collector/status", nil))
	connection = collectorapi.ConnectionStatus{}
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || w.Code != 200 ||
		!connection.Reachable || connection.Authenticated == nil || !*connection.Authenticated || connection.Error != "" {
		t.Fatalf("statistics failure broke connection status: %d %s, %v", w.Code, w.Body, err)
	}
}

func TestLocalCollectorStatusAndSync(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger := slog.New(slog.DiscardHandler)
	favorites := favorites.New(t.Context(), db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "42"}, blockedFavorites{}, logger)
	defer favorites.Close()
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{
		Logger: logger, Favorites: favorites, Status: status.New(db, favorites, pandaban.New(db)), APIToken: "server-secret",
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
	w := request("GET", "/api/collector/status", "")
	var connection collectorapi.ConnectionStatus
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || w.Code != 200 ||
		!connection.Configured || !connection.Reachable || connection.Authenticated == nil || !*connection.Authenticated ||
		connection.Status == nil || !connection.Status.Available ||
		w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "server-secret") {
		t.Fatalf("status: %d %s, %v", w.Code, w.Body, err)
	}
	w = request("GET", "/api/collector/favorites/status", "")
	var favoriteStatus collectorapi.FavoritesStatus
	if err := json.Unmarshal(w.Body.Bytes(), &favoriteStatus); err != nil || w.Code != 200 || len(favoriteStatus.Categories) != 10 {
		t.Fatalf("favorites status: %d %s, %v", w.Code, w.Body, err)
	}
	w = request("GET", "/api/collector/inventory/status", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"gallery_references":0`) {
		t.Fatalf("inventory status: %d %s", w.Code, w.Body)
	}
	w = request("GET", "/api/collector/metadata/status", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"metadata_errors":[]`) {
		t.Fatalf("metadata status: %d %s", w.Code, w.Body)
	}
	for _, category := range []string{"2", "all"} {
		w = request("POST", "/api/collector/favorites/"+category+"/sync", `{"full":true}`)
		if w.Code != 202 {
			t.Fatalf("sync: %d %s", w.Code, w.Body)
		}
	}
	snapshot, err := favorites.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range snapshot.Categories {
		if !(category.State == "running" && category.Full) && !(category.Queued && category.QueuedFull) {
			t.Fatalf("full re-sync not forwarded: %+v", category)
		}
	}
	for _, tc := range []struct{ category, body string }{
		{"10", `{}`}, {"bad", `{}`}, {"2", `{"full":"yes"}`}, {"all", `{"unexpected":true}`},
	} {
		w = request("POST", "/api/collector/favorites/"+tc.category+"/sync", tc.body)
		if w.Code != 400 {
			t.Fatalf("invalid sync accepted: %d %s", w.Code, w.Body)
		}
	}
	w = request("GET", "/api/collector/favorites/download-settings", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"categories":[]`) {
		t.Fatalf("download defaults: %d %s", w.Code, w.Body)
	}
	w = request("PUT", "/api/collector/favorites/download-settings", `{"categories":[9,2]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"categories":[2,9]`) {
		t.Fatalf("save download settings: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{}`, `{"categories":null}`, `{"categories":[-1]}`, `{"categories":[10]}`, `{"categories":[2,2]}`, `{"categories":["2"]}`, `{"categories":[],"unknown":true}`} {
		w = request("PUT", "/api/collector/favorites/download-settings", body)
		if w.Code != 400 {
			t.Fatalf("invalid settings accepted: %d %s", w.Code, w.Body)
		}
	}
	w = request("GET", "/api/collector/favorites/download-settings", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"categories":[2,9]`) {
		t.Fatalf("invalid request changed settings: %d %s", w.Code, w.Body)
	}
	w = request("GET", "/api/collector/favorites/status", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"baseline_state":"collecting"`) || !strings.Contains(w.Body.String(), `"categories":[2,9]`) {
		t.Fatalf("settings missing from status: %d %s", w.Code, w.Body)
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler = NewHandler(&local.App{Logger: logger, Collector: wrong})
	w = request("GET", "/api/collector/status", "")
	connection = collectorapi.ConnectionStatus{}
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || !connection.Reachable ||
		connection.Authenticated == nil || *connection.Authenticated || connection.Status != nil || connection.Error != "collector_unauthorized" {
		t.Fatalf("authentication failure: %s, %v", w.Body, err)
	}
	for _, view := range []string{"favorites", "inventory", "metadata"} {
		w = request("GET", "/api/collector/"+view+"/status", "")
		if w.Code != 502 || !strings.Contains(w.Body.String(), "collector_unauthorized") {
			t.Fatalf("unauthorized %s status: %d %s", view, w.Code, w.Body)
		}
	}
	w = request("POST", "/api/collector/favorites/all/sync", `{}`)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("rejected sync: %d %s", w.Code, w.Body)
	}
	w = request("PUT", "/api/collector/favorites/download-settings", `{"categories":[]}`)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("rejected settings: %d %s", w.Code, w.Body)
	}
	handler = NewHandler(&local.App{Logger: logger})
	w = request("GET", "/api/collector/status", "")
	connection = collectorapi.ConnectionStatus{}
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || connection.Configured || connection.Status != nil {
		t.Fatalf("unconfigured: %s, %v", w.Body, err)
	}
	for _, view := range []string{"favorites", "inventory", "metadata"} {
		w = request("GET", "/api/collector/"+view+"/status", "")
		if w.Code != 503 || !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unconfigured %s status: %d %s", view, w.Code, w.Body)
		}
	}
	w = request("POST", "/api/collector/favorites/all/sync", `{}`)
	if w.Code != 503 {
		t.Fatalf("unconfigured sync: %d", w.Code)
	}
	w = request("GET", "/api/collector/favorites/download-settings", "")
	if w.Code != 503 {
		t.Fatalf("unconfigured settings: %d", w.Code)
	}
}
