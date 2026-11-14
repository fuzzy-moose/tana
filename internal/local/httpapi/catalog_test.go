package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestPandaCatalogThroughLocalProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer collector-secret" {
			t.Error("missing collector authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/catalog":
			if r.URL.Query().Get("q") != `p:"show z$"` || r.URL.Query().Get("cursor") != "next-token" ||
				r.URL.Query().Get("page_size") != "12" || r.URL.Query().Get("include_expunged") != "true" {
				t.Errorf("catalog query changed: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"gallery_id":42,"title":"Show Z","thumbnail_url":"https://images.test/42.jpg","page_count":20,"posted_at":"2026-09-12T00:00:00Z","url":"https://panda.test/g/42/token/"}],"page_size":12,"previous_cursor":"previous-token"}`))
		case "/api/catalog/completions":
			if r.URL.Query().Get("q") != "p:show" || r.URL.Query().Get("cursor") != "6" {
				t.Errorf("completion query changed: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"start":0,"end":6,"items":[{"namespace":"parody","value":"show z","term":"parody:\"show z$\""}]}`))
		case "/api/feed/status", "/api/feed/refresh":
			if r.URL.Path == "/api/feed/refresh" {
				if r.Method != http.MethodPost {
					t.Errorf("refresh method: %s", r.Method)
				}
				w.WriteHeader(http.StatusAccepted)
			}
			_, _ = w.Write([]byte(`{"capture_active":true,"processing_pending":2,"continuity":"overlap","possible_gaps":0}`))
		default:
			t.Errorf("unexpected upstream request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Collector: client})
	for _, tc := range []struct {
		method, path, body, want string
		code                     int
	}{
		{"GET", "/api/collector/catalog?q=p%3A%22show+z%24%22&cursor=next-token&page_size=12&include_expunged=true", "", `"gallery_id":42`, 200},
		{"GET", "/api/collector/catalog/completions?q=p%3Ashow&cursor=6", "", `"namespace":"parody"`, 200},
		{"GET", "/api/collector/feed/status", "", `"processing_pending":2`, 200},
		{"POST", "/api/collector/feed/refresh", `{}`, `"capture_active":true`, 202},
		{"GET", "/api/collector/catalog?page_size=0", "", "invalid_pagination", 400},
		{"GET", "/api/collector/catalog?page_size=101", "", "invalid_pagination", 400},
		{"GET", "/api/collector/catalog?include_expunged=garbage", "", "invalid_query", 400},
		{"GET", "/api/collector/catalog?unseen=garbage", "", "invalid_query", 400},
		{"GET", "/api/collector/catalog/completions?q=x&cursor=-1", "", "invalid_query", 400},
		{"POST", "/api/collector/feed/refresh", `{"unknown":true}`, "invalid", 400},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "collector-secret") {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestCollectorCatalogForwardsUnseen(t *testing.T) {
	var unseen atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unseen.Store(r.URL.Query()["unseen"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"page_size":24}`))
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Collector: client})
	for path, want := range map[string]string{
		"/api/collector/catalog?unseen=true": "true",
		"/api/collector/catalog":             "false",
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if got, _ := unseen.Load().([]string); w.Code != http.StatusOK || !slices.Equal(got, []string{want}) {
			t.Fatalf("%s: %d %s, upstream unseen = %v", path, w.Code, w.Body, got)
		}
	}
}

func TestPandaCatalogProxyErrors(t *testing.T) {
	for _, tc := range []struct {
		upstream, want int
		body, code     string
	}{
		{400, 400, `{"error":"invalid_query"}`, "invalid_query"},
		{400, 400, `{"error":"invalid_category"}`, "invalid_category"},
		{400, 400, `{"error":"invalid_pagination"}`, "invalid_pagination"},
		{401, 502, `{"error":"secret details"}`, "collector_unauthorized"},
		{500, 502, `{"error":"secret details"}`, "collector_unavailable"},
	} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.upstream)
			_, _ = w.Write([]byte(tc.body))
		}))
		client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		HandleCollectorCatalog(client).ServeHTTP(w, httptest.NewRequest("GET", "/?q=unknown:value", nil))
		upstream.Close()
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("upstream %d: got %d %s", tc.upstream, w.Code, w.Body)
		}
	}
	for _, handler := range []http.Handler{HandleCollectorCatalog(nil), HandleCollectorCatalogCompletions(nil), HandleCollectorFeed(nil)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
		}
	}
}

func TestLocalRecordCatalogDisplays(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 3; id++ {
		if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: fmt.Sprintf("Gallery %d", id), Posted: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/displays"
	if w := request(client, "POST", path, `{"gallery_ids":[3,1]}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("record: %d %s", w.Code, w.Body)
	}
	w := request(client, "GET", "/api/collector/catalog", "")
	var page collectorapi.CatalogResult
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("catalog: %d %s, %v", w.Code, w.Body, err)
	}
	displayed := map[int64]bool{}
	for _, item := range page.Items {
		displayed[item.GalleryID] = item.Displayed
	}
	if !maps.Equal(displayed, map[int64]bool{1: true, 2: false, 3: true}) || !strings.Contains(w.Body.String(), `"displayed":true`) {
		t.Fatalf("displayed flags: %s", w.Body)
	}
	tooMany := make([]string, collectorapi.MaxCatalogDisplays+1)
	for i := range tooMany {
		tooMany[i] = strconv.Itoa(i + 1)
	}
	before := requests.Load()
	for _, body := range []string{`{"gallery_ids":[]}`, `{"gallery_ids":[` + strings.Join(tooMany, ",") + `]}`, `{"gallery_ids":[2,0]}`, `{"gallery_ids":[-1]}`} {
		if w := request(client, "POST", path, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_batch") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if w := request(client, "POST", path, `{"gallery_ids":[1],"unknown":true}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_json") {
		t.Fatalf("unknown field: %d %s", w.Code, w.Body)
	}
	if requests.Load() != before {
		t.Fatal("invalid lists reached the collector")
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "POST", path, `{"gallery_ids":[2]}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "POST", path, `{"gallery_ids":[2]}`); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}

func TestLocalCatalogCatchUp(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 3; id++ {
		if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: fmt.Sprintf("Gallery %d", id), Posted: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/catch-up"
	if w := request(client, "POST", path, `{"gallery_id":2}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("catch-up: %d %s", w.Code, w.Body)
	}
	w := request(client, "GET", "/api/collector/catalog?unseen=true", "")
	var page collectorapi.CatalogResult
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("unseen catalog: %d %s, %v", w.Code, w.Body, err)
	}
	if len(page.Items) != 1 || page.Items[0].GalleryID != 3 {
		t.Fatalf("unseen after catch-up: %s", w.Body)
	}
	if w := request(client, "POST", path, `{"gallery_id":99}`); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "gallery_not_found") {
		t.Fatalf("unknown gallery: %d %s", w.Code, w.Body)
	}
	before := requests.Load()
	for _, body := range []string{`{"gallery_id":0}`, `{"gallery_id":-1}`, `{}`} {
		if w := request(client, "POST", path, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_gallery_reference") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if requests.Load() != before {
		t.Fatal("invalid gallery references reached the collector")
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "POST", path, `{"gallery_id":3}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "POST", path, `{"gallery_id":3}`); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}
