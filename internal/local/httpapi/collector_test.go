package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

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
		connection.Status == nil || len(connection.Status.Favorites.Categories) != 10 ||
		w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "server-secret") {
		t.Fatalf("status: %d %s, %v", w.Code, w.Body, err)
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
	w = request("POST", "/api/collector/favorites/all/sync", `{}`)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("rejected sync: %d %s", w.Code, w.Body)
	}
	handler = NewHandler(&local.App{Logger: logger})
	w = request("GET", "/api/collector/status", "")
	connection = collectorapi.ConnectionStatus{}
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil || connection.Configured || connection.Status != nil {
		t.Fatalf("unconfigured: %s, %v", w.Body, err)
	}
	w = request("POST", "/api/collector/favorites/all/sync", `{}`)
	if w.Code != 503 {
		t.Fatalf("unconfigured sync: %d", w.Code)
	}
}
