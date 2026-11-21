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
		{`{"gallery_ids":[1,2]}`, http.StatusNoContent, ""},
		{`{"gallery_ids":[]}`, http.StatusBadRequest, "invalid_batch"},
		{`{"gallery_ids":[` + strings.Repeat("1,", collectorapi.MaxCatalogDisplays) + `1]}`, http.StatusBadRequest, "invalid_batch"},
		{`{"gallery_ids":[1,0]}`, http.StatusBadRequest, "invalid_batch"},
		{`{"gallery_ids":[-1]}`, http.StatusBadRequest, "invalid_batch"},
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
	var displayed string
	if err := db.QueryRow(t.Context(), "SELECT string_agg(gallery_id::text, ',' ORDER BY gallery_id) FROM panda_gallery_displays").Scan(&displayed); err != nil || displayed != "1,2" {
		t.Fatalf("displayed = %q, %v", displayed, err)
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

func TestScorePinValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://favorites.example.test")
	mux := http.NewServeMux()
	mux.Handle("POST /api/catalog/score/pins", HandleSetScorePin(service))
	mux.Handle("DELETE /api/catalog/score/pins/{id}", HandleRemoveScorePin(service))
	const path = "/api/catalog/score/pins"
	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   int
		code   string
	}{
		{http.MethodPost, path, `{"kind":"uploader","value":"Some One"}`, http.StatusBadRequest, "invalid_pin"},
		{http.MethodPost, path, `{"kind":"uploader","value":"Some One","weight":null}`, http.StatusBadRequest, "invalid_pin"},
		{http.MethodPost, path, `{"kind":"title","value":"x","weight":1}`, http.StatusBadRequest, "invalid_pin"},
		{http.MethodPost, path, `{"kind":"tag","value":"female:glasses","weight":1}`, http.StatusNotFound, "tag_not_found"},
		{http.MethodPost, path, `{"kind":"uploader","value":"Some One","weight":1,"unknown":true}`, http.StatusBadRequest, "invalid_json"},
		{http.MethodDelete, path + "/3", "", http.StatusNoContent, ""},
		{http.MethodDelete, path + "/0", "", http.StatusBadRequest, "invalid_pin"},
		{http.MethodDelete, path + "/x", "", http.StatusBadRequest, "invalid_pin"},
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
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"kind":"uploader","value":" Some One ","weight":-2.5}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	var pin map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &pin); err != nil || response.Code != http.StatusOK {
		t.Fatalf("response = %d %s, %v", response.Code, response.Body.String(), err)
	}
	if id, _ := pin["id"].(float64); id <= 0 || len(pin) != 4 || pin["kind"] != "uploader" || pin["label"] != "Some One" || pin["weight"] != -2.5 {
		t.Fatalf("pin = %v", pin)
	}
}

func TestScoreTagWeightsRoute(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	handler := HandleScoreTagWeights(catalog.New(db, "https://favorites.example.test"))
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"tags":["female:glasses"]}`, http.StatusOK},
		{`{"tags":["female:glasses"],"unknown":true}`, http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/catalog/score/tags", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("%s: response = %d %s", tc.body, response.Code, response.Body.String())
		}
		if tc.want != http.StatusOK {
			var result map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["error"] != "invalid_json" {
				t.Fatalf("response = %s, %v", response.Body.String(), err)
			}
			continue
		}
		var result struct {
			Active *bool                        `json:"active"`
			Tags   []map[string]json.RawMessage `json:"tags"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Active == nil || *result.Active || len(result.Tags) != 1 {
			t.Fatalf("response = %s, %v", response.Body.String(), err)
		}
		entry := result.Tags[0]
		if len(entry) != 3 || string(entry["tag"]) != `"female:glasses"` || string(entry["known"]) != "false" || string(entry["weight"]) != "null" {
			t.Fatalf("entry = %s", response.Body.String())
		}
	}
}

func TestAuthorBonusValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	handler := HandleSetAuthorBonus(catalog.New(db, "https://favorites.example.test"))
	for _, tc := range []struct {
		body string
		want int
		code string
	}{
		{`{"bonus":1.5}`, http.StatusNoContent, ""},
		{`{"bonus":null}`, http.StatusNoContent, ""},
		{`{}`, http.StatusNoContent, ""},
		{`{"bonus":0}`, http.StatusBadRequest, "invalid_author_bonus"},
		{`{"bonus":-1}`, http.StatusBadRequest, "invalid_author_bonus"},
		{`{"bonus":2e300}`, http.StatusBadRequest, "invalid_author_bonus"},
		{`{"bonus":1,"unknown":true}`, http.StatusBadRequest, "invalid_json"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/api/catalog/score/author-bonus", strings.NewReader(tc.body))
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

func TestScorePreviewValidation(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	handler := HandlePreviewScoreModel(catalog.New(db, "https://favorites.example.test"))
	for _, tc := range []struct {
		body string
		want int
		code string
	}{
		{`{"default_query":"","default_categories":[]}`, http.StatusOK, ""},
		{`{"default_query":"","default_categories":[],"candidate_id":7}`, http.StatusOK, ""},
		{`{"default_categories":["unknown"]}`, http.StatusBadRequest, "invalid_category"},
		{`{"default_query":"\"unfinished"}`, http.StatusBadRequest, "invalid_query"},
		{`{"unknown":1}`, http.StatusBadRequest, "invalid_json"},
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
				string(result["no_model"]) != `"no_favorites"` {
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
	if err := db.QueryRow(t.Context(), `INSERT INTO panda_score_models (calculated_at, favorites_in_scope, catalog_in_scope)
		VALUES (now(), 0, 0) RETURNING id`).Scan(&candidate); err != nil {
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
