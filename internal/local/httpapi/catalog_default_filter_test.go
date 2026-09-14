package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/catalogfilter"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func TestCatalogDefaultFilterPersistenceAndBypass(t *testing.T) {
	var forwarded url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		if forwarded.Get("q") == "invalid:" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_query"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[],"page_size":1}`))
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := &local.App{Logger: slog.New(slog.DiscardHandler), Collector: client, CatalogFilter: catalogfilter.New(db)}
	handler := NewHandler(app)
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		return w
	}

	request("PUT", "/api/panda/default-filter", `{"query":"-l:japanese$","categories":["Manga","Doujinshi"]}`, 200)
	if forwarded.Get("q") != "-l:japanese$" {
		t.Fatalf("default query was not validated: %v", forwarded)
	}
	// A separately constructed handler sees the same shared setting.
	handler = NewHandler(&local.App{Logger: app.Logger, Collector: client, CatalogFilter: catalogfilter.New(db)})
	w := request("GET", "/api/panda/default-filter", "", 200)
	var saved catalogfilter.Filter
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil || saved.Query != "-l:japanese$" || len(saved.Categories) != 2 {
		t.Fatalf("saved default: %s (%v)", w.Body, err)
	}
	request("GET", "/api/collector/catalog?q=~cat+~dog&category=manga", "", 200)
	if forwarded.Get("q") != "~cat ~dog" || forwarded.Get("default_q") != "-l:japanese$" ||
		!slices.Equal(forwarded["category"], []string{"manga"}) || !slices.Equal(forwarded["default_category"], saved.Categories) {
		t.Fatalf("current and default filters not kept separate: %v", forwarded)
	}
	request("GET", "/api/collector/catalog?q=cat&bypass_default=true", "", 200)
	if forwarded.Get("default_q") != "" || len(forwarded["default_category"]) != 0 || forwarded.Get("q") != "cat" {
		t.Fatalf("bypass did not preserve current search: %v", forwarded)
	}
	request("GET", "/api/collector/catalog?bypass_default=invalid", "", 400)
	request("PUT", "/api/panda/default-filter", `{"query":"invalid:","categories":[]}`, 400)
	request("PUT", "/api/panda/default-filter", `{"query":"","categories":["unsupported"]}`, 400)
	w = request("GET", "/api/panda/default-filter", "", 200)
	if !strings.Contains(w.Body.String(), "-l:japanese$") {
		t.Fatalf("invalid save changed default: %s", w.Body)
	}
	request("PUT", "/api/panda/default-filter", `{"query":"","categories":[]}`, 200)
	request("GET", "/api/collector/catalog", "", 200)
	if forwarded.Get("default_q") != "" || len(forwarded["default_category"]) != 0 {
		t.Fatalf("cleared default still applied: %v", forwarded)
	}
}
