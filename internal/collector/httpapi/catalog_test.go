package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestCatalogPaginationAndQueryValidation(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := catalog.New(db, "https://favorites.example.test")
	for _, tc := range []struct {
		path string
		code string
	}{
		{"/api/catalog", ""},
		{"/api/catalog?page=0", "invalid_pagination"},
		{"/api/catalog?page_size=101", "invalid_pagination"},
		{"/api/catalog?page_size=words", "invalid_pagination"},
		{"/api/catalog?include_expunged=maybe", "invalid_query"},
		{"/api/catalog?q=unknown:value", "invalid_query"},
		{"/api/catalog?q=%22unfinished", "invalid_query"},
		{"/api/catalog/completions?q=a:a&cursor=3", ""},
		{"/api/catalog/completions?q=a:a&cursor=4", "invalid_query"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle("GET /api/catalog", HandleCatalog(service))
			mux.Handle("GET /api/catalog/completions", HandleCompleteCatalog(service))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if tc.code == "" {
				if response.Code != http.StatusOK {
					t.Fatalf("response = %d %s", response.Code, response.Body.String())
				}
				if tc.path == "/api/catalog" {
					var result collectorapi.CatalogResult
					if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Items == nil || result.Total != 0 || result.Page != 1 || result.PageSize != 24 || result.TotalPages != 1 {
						t.Fatalf("empty catalog = %+v, %v", result, err)
					}
				}
				return
			}
			var result map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusBadRequest || result["error"] != tc.code {
				t.Fatalf("response = %d %s, %v", response.Code, response.Body.String(), err)
			}
		})
	}
}
