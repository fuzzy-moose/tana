package httpapi

import (
	"encoding/json"
	"fmt"
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
		{"/api/catalog?verdict=interesting", ""},
		{"/api/catalog?verdict=maybe", "invalid_query"},
		{"/api/catalog?verdict=interesting&unseen=true", "invalid_query"},
		{"/api/catalog?score_filter=bogus", "invalid_query"},
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
		{`{"gallery_ids":[1],"score_filter":"hidden"}`, http.StatusNoContent, ""},
		{`{"gallery_ids":[]}`, http.StatusBadRequest, "invalid_batch"},
		{`{"gallery_ids":[1],"score_filter":"bogus"}`, http.StatusBadRequest, "invalid_batch"},
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

func TestCatalogVerdictValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://favorites.example.test")
	mux := http.NewServeMux()
	mux.Handle("PUT /api/catalog/verdicts/{id}", HandleSetCatalogVerdict(service))
	mux.Handle("DELETE /api/catalog/verdicts/{id}", HandleRemoveCatalogVerdict(service))
	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   int
		code   string
	}{
		{http.MethodPut, "/api/catalog/verdicts/7", `{"verdict":"interesting"}`, http.StatusNoContent, ""},
		{http.MethodPut, "/api/catalog/verdicts/7", `{"verdict":"maybe"}`, http.StatusBadRequest, "invalid_verdict"},
		{http.MethodPut, "/api/catalog/verdicts/7", `{"verdict":"interesting","unknown":true}`, http.StatusBadRequest, "invalid_json"},
		{http.MethodPut, "/api/catalog/verdicts/0", `{"verdict":"interesting"}`, http.StatusBadRequest, "invalid_gallery_reference"},
		{http.MethodPut, "/api/catalog/verdicts/x", `{"verdict":"interesting"}`, http.StatusBadRequest, "invalid_gallery_reference"},
		{http.MethodDelete, "/api/catalog/verdicts/7", "", http.StatusNoContent, ""},
		{http.MethodDelete, "/api/catalog/verdicts/0", "", http.StatusBadRequest, "invalid_gallery_reference"},
		{http.MethodDelete, "/api/catalog/verdicts/x", "", http.StatusBadRequest, "invalid_gallery_reference"},
	} {
		t.Run(tc.method+" "+tc.path+" "+tc.body, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
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

func TestScoreOverrideValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://favorites.example.test")
	mux := http.NewServeMux()
	mux.Handle("POST /api/catalog/score/overrides", HandleSetScoreOverride(service))
	mux.Handle("DELETE /api/catalog/score/overrides/{id}", HandleRemoveScoreOverride(service))
	const path = "/api/catalog/score/overrides"
	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   int
		code   string
	}{
		{http.MethodPost, path, `{"kind":"uploader","value":"Some One","rule":"always_hide"}`, http.StatusNoContent, ""},
		{http.MethodPost, path, `{"kind":"category","value":"comics","rule":"always_hide"}`, http.StatusBadRequest, "invalid_override"},
		{http.MethodPost, path, `{"kind":"uploader","value":"Some One","rule":"maybe"}`, http.StatusBadRequest, "invalid_override"},
		{http.MethodPost, path, `{"kind":"tag","value":"female:glasses","rule":"never_hide"}`, http.StatusNotFound, "tag_not_found"},
		{http.MethodPost, path, `{"kind":"uploader","value":"Some One","rule":"always_hide","unknown":true}`, http.StatusBadRequest, "invalid_json"},
		{http.MethodDelete, path + "/3", "", http.StatusNoContent, ""},
		{http.MethodDelete, path + "/0", "", http.StatusBadRequest, "invalid_override"},
		{http.MethodDelete, path + "/x", "", http.StatusBadRequest, "invalid_override"},
	} {
		t.Run(tc.method+" "+tc.path+" "+tc.body, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
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

func TestScorePreviewValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	handler := HandlePreviewScoreModel(catalog.New(db, "https://favorites.example.test"))
	for _, tc := range []struct {
		body string
		want int
		code string
	}{
		{`{"favorites":false,"default_query":"","default_categories":[]}`, http.StatusOK, ""},
		{`{"favorites":null,"default_categories":["unknown"]}`, http.StatusBadRequest, "invalid_category"},
		{`{"default_query":"\"unfinished"}`, http.StatusBadRequest, "invalid_query"},
		{`{"favorites":true,"unknown":1}`, http.StatusBadRequest, "invalid_json"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/catalog/score/preview", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if tc.code != "" {
				if string(result["error"]) != `"`+tc.code+`"` {
					t.Fatalf("response = %s", response.Body.String())
				}
				return
			}
			if result["evidence"] == nil || string(result["overrides"]) != "[]" || string(result["candidate"]) != "null" ||
				string(result["no_model"]) != `"no_verdicts"` {
				t.Fatalf("preview = %s", response.Body.String())
			}
		})
	}
}

func TestScoreModelValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://favorites.example.test")
	mux := http.NewServeMux()
	mux.Handle("POST /api/catalog/score/apply", HandleApplyScoreModel(service))
	mux.Handle("PUT /api/catalog/score/threshold", HandleSetScoreThreshold(service))
	mux.Handle("DELETE /api/catalog/score/model", HandleRemoveScoreModel(service))
	var candidate int64
	if err := db.QueryRow(t.Context(), `INSERT INTO panda_score_models (favorites, calculated_at) VALUES (false, now())
		RETURNING id`).Scan(&candidate); err != nil {
		t.Fatal(err)
	}
	const apply, threshold, model = "/api/catalog/score/apply", "/api/catalog/score/threshold", "/api/catalog/score/model"
	// The cases run in order: the apply in the middle makes the model active.
	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   int
		code   string
	}{
		{http.MethodPut, threshold, `{"threshold_tenths":-5}`, http.StatusConflict, "score_model_not_active"},
		{http.MethodPost, apply, fmt.Sprintf(`{"candidate_id":%d,"threshold_tenths":0}`, candidate+1), http.StatusConflict, "score_candidate_not_found"},
		{http.MethodPost, apply, fmt.Sprintf(`{"candidate_id":%d,"threshold_tenths":2147483648}`, candidate), http.StatusBadRequest, "invalid_threshold"},
		{http.MethodPost, apply, fmt.Sprintf(`{"candidate_id":%d,"threshold_tenths":0,"unknown":1}`, candidate), http.StatusBadRequest, "invalid_json"},
		{http.MethodPost, apply, fmt.Sprintf(`{"candidate_id":%d,"threshold_tenths":0}`, candidate), http.StatusNoContent, ""},
		{http.MethodPut, threshold, `{"threshold_tenths":-2147483649}`, http.StatusBadRequest, "invalid_threshold"},
		{http.MethodPut, threshold, `{"threshold_tenths":-5,"unknown":1}`, http.StatusBadRequest, "invalid_json"},
		{http.MethodPut, threshold, `{"threshold_tenths":-5}`, http.StatusNoContent, ""},
		{http.MethodDelete, model, "", http.StatusNoContent, ""},
		{http.MethodDelete, model, "", http.StatusNoContent, ""},
	} {
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("%s %s %s: response = %d %s", tc.method, tc.path, tc.body, response.Code, response.Body.String())
		}
		if tc.code == "" {
			if response.Body.Len() != 0 {
				t.Fatalf("%s %s %s: body = %q", tc.method, tc.path, tc.body, response.Body.String())
			}
			continue
		}
		var result map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["error"] != tc.code {
			t.Fatalf("%s %s %s: response = %s, %v", tc.method, tc.path, tc.body, response.Body.String(), err)
		}
	}
}
