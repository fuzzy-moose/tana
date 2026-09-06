package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestAuthenticationProtectsEveryRouteExceptHealth(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(&collector.App{
		Logger:   slog.New(slog.NewJSONHandler(&logs, nil)),
		APIToken: "collector-secret",
	})
	for _, path := range []string{"/api/metadata/lookup", "/api/metadata/fetches", "/api/metadata/fetches/job", "/api/favorites/2/sync", "/missing", "/healthz/"} {
		for _, auth := range []string{"", "Bearer wrong", "Basic collector-secret", "Bearer", "Bearer  collector-secret"} {
			r := httptest.NewRequest(http.MethodPost, path+"?token=collector-secret", nil)
			r.Header.Set("Authorization", auth)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
				t.Fatalf("%s with %q: %d %s", path, auth, w.Code, w.Body)
			}
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	r.Header.Add("Authorization", "Bearer collector-secret")
	r.Header.Add("Authorization", "Bearer collector-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("duplicate credentials accepted: %d", w.Code)
	}
	for _, auth := range []string{"Bearer collector-secret", "bearer collector-secret"} {
		r := httptest.NewRequest(http.MethodGet, "/missing", nil)
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("valid credentials rejected: %d", w.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, "/healthz", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("anonymous health: %d", w.Code)
		}
	}
	if strings.Contains(logs.String(), "collector-secret") {
		t.Fatal("credentials leaked into request logs")
	}
	r = httptest.NewRequest(http.MethodGet, "/missing", nil)
	r.Header.Set("Authorization", "Bearer ")
	w = httptest.NewRecorder()
	NewHandler(&collector.App{Logger: slog.New(slog.DiscardHandler)}).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("empty configured token opened API access")
	}
}

type waitingFavoritesClient struct{}

func (waitingFavoritesClient) GetFavoritesPage(ctx context.Context, category int, next string) (panda.FavoritesPage, error) {
	<-ctx.Done()
	return panda.FavoritesPage{}, ctx.Err()
}

func TestFavoritesAPIEnqueuesWithoutJobResponse(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger := slog.New(slog.DiscardHandler)
	service := favorites.New(t.Context(), db, panda.FavoritesConfig{URL: "https://panda.test", AccountKey: "42"}, waitingFavoritesClient{}, logger)
	defer service.Close()
	handler := NewHandler(&collector.App{Logger: logger, Favorites: service, APIToken: "test-token"})
	for _, body := range []string{`{}`, `{"full":true}`, `{"full":false}`} {
		w := apiRequest(handler, http.MethodPost, "/api/favorites/2/sync", body)
		if w.Code != http.StatusAccepted || w.Body.Len() != 0 || w.Header().Get("Location") != "" {
			t.Fatalf("response: %d %s", w.Code, w.Body)
		}
	}
	for _, tc := range []struct{ category, body string }{
		{"10", `{}`}, {"-1", `{}`}, {"bad", `{}`}, {"2", `{"full":"yes"}`}, {"2", `{"unknown":true}`},
	} {
		w := apiRequest(handler, http.MethodPost, "/api/favorites/"+tc.category+"/sync", tc.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid request accepted: %d %s", w.Code, w.Body)
		}
	}
}

func pausedHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // HTTP admission and polling do not depend on a running worker.
	logger := slog.New(slog.DiscardHandler)
	service := metadata.New(ctx, db, nil, logger)
	t.Cleanup(service.Close)
	return NewHandler(&collector.App{Logger: logger, Metadata: service, APIToken: "test-token"}), db
}

func apiRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestBatchAPIStoredLookupSubmissionAndPolling(t *testing.T) {
	handler, db := pausedHandler(t)
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (1, 'token1');
		INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, '{"gid":1,"token":"token1","title":"Saved"}', 1000);`); err != nil {
		t.Fatal(err)
	}
	w := apiRequest(handler, http.MethodPost, "/api/metadata/lookup", `{"gallery_ids":[1,2]}`)
	var result collectorapi.LookupResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK ||
		len(result.Galleries) != 1 || result.Galleries[0].Metadata.Title != "Saved" || result.Galleries[0].RefreshedAt.UnixMilli() != 1000 ||
		len(result.UnknownIDs) != 1 || result.UnknownIDs[0] != 2 {
		t.Fatalf("lookup: %d %s, %v", w.Code, w.Body, err)
	}
	refs := make([]panda.GalleryRef, metadata.MaxFetchSize)
	for i := range refs {
		refs[i] = panda.GalleryRef{ID: int64(i + 1), Token: fmt.Sprintf("token%d", i+1)}
	}
	refs[0].Token = "conflicting"
	body, err := json.Marshal(map[string]any{"galleries": refs})
	if err != nil {
		t.Fatal(err)
	}
	w = apiRequest(handler, http.MethodPost, "/api/metadata/fetches", string(body))
	var job metadata.FetchJob
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil || w.Code != http.StatusAccepted || len(job.Entries) != metadata.MaxFetchSize ||
		job.Entries[0].Error != "token_conflict" || job.Entries[1].Status != "pending" {
		t.Fatalf("submission: %d %s, %v", w.Code, w.Body, err)
	}
	location := w.Header().Get("Location")
	if location != "/api/metadata/fetches/"+job.ID {
		t.Fatalf("polling location: %q", location)
	}
	w = apiRequest(handler, http.MethodGet, location, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"pending"`) || strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("poll: %d %s", w.Code, w.Body)
	}
	w = apiRequest(handler, http.MethodGet, "/api/metadata/fetches/unknown", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown job: %d", w.Code)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM gallery_refs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("submission admitted unvalidated references: %d, %v", count, err)
	}
}

func TestBatchAPIRejectsInvalidRequests(t *testing.T) {
	handler, _ := pausedHandler(t)
	oversized := make([]panda.GalleryRef, metadata.MaxFetchSize+1)
	for i := range oversized {
		oversized[i] = panda.GalleryRef{ID: int64(i + 1), Token: "token"}
	}
	body, err := json.Marshal(map[string]any{"galleries": oversized})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body string }{
		{"lookup", `{"gallery_ids":[]}`},
		{"lookup", `{"gallery_ids":[1,1]}`},
		{"lookup", `{"gallery_ids":[0]}`},
		{"lookup", `{"gallery_ids":[1],"unknown":true}`},
		{"lookup", `{"gallery_ids":[1]} {}`},
		{"fetches", `{"galleries":[{"gid":1}]}`},
		{"fetches", `{"galleries":[{"gid":1,"token":"a"},{"gid":1,"token":"b"}]}`},
		{"fetches", string(body)},
		{"fetches", `{"galleries":[{"gid":1,"token":"` + strings.Repeat("x", 1024*1024) + `"}]}`},
	} {
		w := apiRequest(handler, http.MethodPost, "/api/metadata/"+tc.path, tc.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid %s request: %d %s", tc.path, w.Code, w.Body)
		}
	}
	w := apiRequest(handler, http.MethodGet, "/api/metadata/lookup", "")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "POST" {
		t.Fatalf("wrong method: %d %v", w.Code, w.Header())
	}
}
