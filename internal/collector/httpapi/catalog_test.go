package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestCatalogPaginationAndQueryValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://favorites.example.test")
	for _, tc := range []struct {
		path string
		code string
	}{
		{"/api/catalog", ""},
		{"/api/catalog?cursor=invalid", "invalid_pagination"},
		{"/api/catalog?page_size=101", "invalid_pagination"},
		{"/api/catalog?page_size=words", "invalid_pagination"},
		{"/api/catalog?include_expunged=maybe", "invalid_query"},
		{"/api/catalog?unseen=maybe", "invalid_query"},
		{"/api/catalog?unseen=true", ""},
		{"/api/catalog?q=unknown:value", "invalid_query"},
		{"/api/catalog?q=%22unfinished", "invalid_query"},
		{"/api/catalog?default_q=%22unfinished", "invalid_query"},
		{"/api/catalog?category=manga&category=doujinshi", ""},
		{"/api/catalog?category=unknown", "invalid_category"},
		{"/api/catalog?default_category=unknown", "invalid_category"},
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
					if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Items == nil || len(result.Items) != 0 || result.PageSize != 24 || result.NextCursor != "" || result.PreviousCursor != "" {
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

func TestRecordCatalogDisplaysValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	handler := HandleRecordCatalogDisplays(catalog.New(db, "https://favorites.example.test"))
	for _, tc := range []struct {
		body string
		want int
		code string
	}{
		{`{"gallery_ids":[2,1]}`, http.StatusNoContent, ""},
		{`{"gallery_ids":[]}`, http.StatusBadRequest, "invalid_batch"},
		{`{"gallery_ids":[1],"unknown":true}`, http.StatusBadRequest, "invalid_json"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/catalog/displays", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if tc.code == "" {
				if response.Body.Len() != 0 {
					t.Fatalf("body = %q", response.Body.String())
				}
				return
			}
			var result map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["error"] != tc.code {
				t.Fatalf("response = %s, %v", response.Body.String(), err)
			}
		})
	}
}

func TestCatalogCatchUpValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	tx := pgtest.Begin(t, db)
	if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES (1, 'token')"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: 1, Title: "Gallery", Posted: 10}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	handler := HandleCatalogCatchUp(catalog.New(db, "https://favorites.example.test"))
	for _, tc := range []struct {
		body string
		want int
		code string
	}{
		{`{"gallery_id":1}`, http.StatusNoContent, ""},
		{`{"gallery_id":2}`, http.StatusNotFound, "gallery_not_found"},
		{`{"gallery_id":0}`, http.StatusBadRequest, "invalid_gallery_reference"},
		{`{"gallery_id":1,"unknown":true}`, http.StatusBadRequest, "invalid_json"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/catalog/catch-up", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if tc.code == "" {
				if response.Body.Len() != 0 {
					t.Fatalf("body = %q", response.Body.String())
				}
				return
			}
			var result map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["error"] != tc.code {
				t.Fatalf("response = %s, %v", response.Body.String(), err)
			}
		})
	}
}
