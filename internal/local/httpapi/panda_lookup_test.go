package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestPandaLookupThroughLocalProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer collector-secret" {
			t.Error("missing collector authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /api/catalog/lookup":
			var ref panda.GalleryRef
			if err := json.NewDecoder(r.Body).Decode(&ref); err != nil || ref.ID != 42 {
				t.Errorf("lookup reference: %+v, %v", ref, err)
			}
			if ref.Token == "" {
				_, _ = w.Write([]byte(`{"gid":42,"token":"known-token","url":"https://panda.test/g/42/known-token/","metadata":{"gid":42,"token":"known-token","title":"Retained","expunged":true},"refreshed_at":"2026-09-12T00:00:00Z"}`))
			} else {
				if ref.Token != "pasted-token" {
					t.Errorf("pasted token changed: %q", ref.Token)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"gid":42,"token":"pasted-token","url":"https://panda.test/g/42/pasted-token/","unverified":true,"fetch_job":{"id":"job-42","status":"pending","created_at":"2026-09-12T00:00:00Z","entries":[{"gid":42,"status":"pending"}]}}`))
			}
		case "GET /api/metadata/fetches/job-42":
			_, _ = w.Write([]byte(`{"id":"job-42","status":"completed","created_at":"2026-09-12T00:00:00Z","completed_at":"2026-09-12T00:01:00Z","entries":[{"gid":42,"status":"successful","refreshed_at":"2026-09-12T00:01:00Z"}]}`))
		default:
			t.Errorf("unexpected collector request: %s %s", r.Method, r.URL)
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
		status                   int
	}{
		{"POST", "/api/collector/catalog/lookup", `{"gid":42}`, `"expunged":true`, 200},
		{"POST", "/api/collector/catalog/lookup", `{"gid":42,"token":"pasted-token"}`, `"unverified":true`, 202},
		{"GET", "/api/collector/metadata/fetches/job-42", "", `"status":"successful"`, 200},
		{"POST", "/api/collector/catalog/lookup", `{"gid":42,"unknown":true}`, `"error":"invalid_json"`, 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "collector-secret") {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestPandaLookupProxyDistinguishesMissingDataFromOutages(t *testing.T) {
	for _, tc := range []struct {
		name, upstreamCode, wantCode string
		upstreamStatus, wantStatus   int
		fetch                        bool
	}{
		{"unknown gallery", "gallery_not_found", "gallery_not_found", 404, 404, false},
		{"missing collector endpoint", "", "collector_unavailable", 404, 502, false},
		{"invalid reference", "invalid_gallery_reference", "invalid_gallery_reference", 400, 400, false},
		{"expired fetch", "not_found", "metadata_fetch_not_found", 404, 404, true},
		{"unauthorized", "secret details", "collector_unauthorized", 401, 502, false},
		{"lookup failure", "secret details", "collector_unavailable", 500, 502, false},
		{"poll failure", "secret details", "collector_unavailable", 500, 502, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.upstreamStatus)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": tc.upstreamCode})
			}))
			defer upstream.Close()
			client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
			if err != nil {
				t.Fatal(err)
			}
			handler := HandleCollectorGalleryLookup(client)
			r := httptest.NewRequest("POST", "/", strings.NewReader(`{"gid":42}`))
			r.Header.Set("Content-Type", "application/json")
			if tc.fetch {
				handler = HandleCollectorMetadataFetch(client)
				r.SetPathValue("id", "job-42")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.wantStatus || !strings.Contains(w.Body.String(), tc.wantCode) || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
		})
	}
	for _, handler := range []http.Handler{HandleCollectorGalleryLookup(nil), HandleCollectorMetadataFetch(nil)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
		}
	}
}
