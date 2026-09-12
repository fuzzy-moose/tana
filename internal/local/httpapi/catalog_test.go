package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
)

func TestPandaCatalogThroughLocalProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer collector-secret" {
			t.Error("missing collector authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/catalog":
			if r.URL.Query().Get("q") != `p:"show z$"` || r.URL.Query().Get("page") != "2" ||
				r.URL.Query().Get("page_size") != "12" || r.URL.Query().Get("include_expunged") != "true" {
				t.Errorf("catalog query changed: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"gallery_id":42,"title":"Show Z","thumbnail_url":"https://images.test/42.jpg","page_count":20,"posted_at":"2026-09-12T00:00:00Z","url":"https://panda.test/g/42/token/"}],"total":13,"page":2,"page_size":12,"total_pages":2}`))
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
		{"GET", "/api/collector/catalog?q=p%3A%22show+z%24%22&page=2&page_size=12&include_expunged=true", "", `"gallery_id":42`, 200},
		{"GET", "/api/collector/catalog/completions?q=p%3Ashow&cursor=6", "", `"namespace":"parody"`, 200},
		{"GET", "/api/collector/feed/status", "", `"processing_pending":2`, 200},
		{"POST", "/api/collector/feed/refresh", `{}`, `"capture_active":true`, 202},
		{"GET", "/api/collector/catalog?page=0", "", "invalid_pagination", 400},
		{"GET", "/api/collector/catalog?page_size=101", "", "invalid_pagination", 400},
		{"GET", "/api/collector/catalog?include_expunged=garbage", "", "invalid_query", 400},
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

func TestPandaCatalogProxyErrors(t *testing.T) {
	for _, tc := range []struct {
		upstream, want int
		body, code     string
	}{
		{400, 400, `{"error":"invalid_query"}`, "invalid_query"},
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
