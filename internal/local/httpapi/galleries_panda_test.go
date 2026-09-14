package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func TestGalleryPandaFiltersUseCurrentCollectorFacts(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	galleries := gallery.NewSQLiteRepository(db)
	l, err := library.NewSQLiteRepository(db).Create(t.Context(), "Library", filepath.Join(t.TempDir(), "books"))
	if err != nil {
		t.Fatal(err)
	}
	sources := source.NewSQLiteRepository(db)
	for _, name := range []string{"A [11]", "B [22]", "C [11]"} {
		s, err := sources.Create(t.Context(), l.ID, name, source.Directory, []string{"1.jpg"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := galleries.CreateFromSource(t.Context(), s.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := galleries.Create(t.Context(), "Manual [11]", nil); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	var removed, unavailable atomic.Bool
	var expectedIDs atomic.Value
	expectedIDs.Store([]int64{11, 22})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/catalog/facts" {
			t.Errorf("unexpected collector request: %s %s", r.Method, r.URL)
		}
		if unavailable.Load() {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
		var input struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !reflect.DeepEqual(input.GalleryIDs, expectedIDs.Load()) {
			t.Errorf("collector IDs: %v, %v", input.GalleryIDs, err)
		}
		newer, older := int64(200), int64(100)
		facts := []collectorapi.CatalogFact{
			{GalleryID: 11, Category: "manga", FavoritedAt: &newer},
			{GalleryID: 22, Category: "doujinshi", FavoritedAt: &older},
		}
		if removed.Load() {
			facts[0].FavoritedAt = nil
		}
		facts = slices.DeleteFunc(facts, func(f collectorapi.CatalogFact) bool { return !slices.Contains(input.GalleryIDs, f.GalleryID) })
		_ = json.NewEncoder(w).Encode(facts)
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	handler := NewHandler(&local.App{Logger: logger, Galleries: galleries, Collector: client})
	listing := func(path string) gallery.Listing {
		t.Helper()
		var result gallery.Listing
		if err := json.Unmarshal(request(t, handler, "GET", path, "", 200).Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := listing("/api/galleries?sort=title"); result.Total != 4 || calls.Load() != 0 {
		t.Fatalf("ordinary browsing: %+v, collector calls %d", result, calls.Load())
	}
	result := listing("/api/galleries?category=manga&category=doujinshi&sort=favorited_asc&page=2&page_size=1")
	if result.Total != 3 || result.Page != 2 || len(result.Items) != 1 || result.Items[0].Title != "A [11]" {
		t.Fatalf("filtered second page: %+v", result)
	}
	removed.Store(true)
	result = listing("/api/galleries?sort=favorited_desc&page_size=2")
	if result.Total != 4 || len(result.Items) != 2 || result.Items[0].Title != "B [22]" || result.Items[1].Title != "A [11]" {
		t.Fatalf("removed favorite timestamp remained in use: %+v", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("collector calls: %d", calls.Load())
	}
	for _, query := range []string{"sort=unknown", "category=unknown", "category=manga&category=unknown"} {
		request(t, handler, "GET", "/api/galleries?"+query, "", 400)
	}
	if calls.Load() != 2 {
		t.Fatalf("invalid filters called collector: %d", calls.Load())
	}
	expectedIDs.Store([]int64{22})
	result = listing("/api/galleries?q=title:B&category=doujinshi&sort=favorited_desc")
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Title != "B [22]" || calls.Load() != 3 {
		t.Fatalf("searched listing: %+v, collector calls %d", result, calls.Load())
	}
	for _, search := range []string{"Manual", "missing"} {
		result = listing("/api/galleries?q=title:" + search + "&sort=favorited_desc")
		if calls.Load() != 3 {
			t.Fatalf("search without Panda candidates called collector: %+v, calls %d", result, calls.Load())
		}
	}
	request(t, handler, "GET", "/api/galleries?q=unknown:value&sort=favorited_desc", "", 400)
	if calls.Load() != 3 {
		t.Fatalf("invalid search called collector: %d", calls.Load())
	}
	unavailable.Store(true)
	if result := listing("/api/galleries"); result.Total != 4 || calls.Load() != 3 {
		t.Fatalf("ordinary browsing depends on collector: %+v, calls %d", result, calls.Load())
	}
	if response := request(t, handler, "GET", "/api/galleries?category=manga", "", 502); !strings.Contains(response.Body.String(), "collector_unavailable") {
		t.Fatalf("collector error: %s", response.Body)
	}
	unconfigured := NewHandler(&local.App{Logger: logger, Galleries: galleries})
	request(t, unconfigured, "GET", "/api/galleries?sort=favorited_asc", "", 503)
}
