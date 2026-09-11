package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type blockedArchive struct{}

func (blockedArchive) GetArchiveURL(ctx context.Context, _ panda.GalleryRef) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type downloadRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *downloadRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestLocalCollectorDownloads(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	logger := slog.New(slog.DiscardHandler)
	service, err := downloads.New(t.Context(), db, dir, blockedArchive{}, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Downloads: service, APIToken: "server-secret"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") == "Bearer browser-secret" {
			t.Error("browser credentials forwarded to collector")
		}
		w.Header().Set("Set-Cookie", "upstream-secret=hidden")
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: logger, Collector: client})
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Cookie", "browser-secret=hidden")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("collector secret exposed: %v %s", w.Header(), w.Body)
		}
		return w
	}
	for range 2 {
		w := request("POST", "/api/collector/downloads", `{"gid":42,"token":"gallery-secret"}`)
		if w.Code != 202 || w.Header().Get("Location") != "/api/collector/downloads/42" {
			t.Fatalf("submit: %d %s", w.Code, w.Body)
		}
	}
	w := request("GET", "/api/collector/downloads?limit=26&offset=0", "")
	var list collectorapi.DownloadList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != 200 || len(list.Jobs) != 1 || list.Jobs[0].GalleryID != 42 {
		t.Fatalf("list: %d %s, %v", w.Code, w.Body, err)
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"GET", "/42", "", 200, ""},
		{"GET", "?offset=1", "", 200, ""},
		{"GET", "?limit=101", "", 400, "invalid_pagination"},
		{"GET", "?offset=-1", "", 400, "invalid_pagination"},
		{"GET", "/bad", "", 400, "invalid_gallery_reference"},
		{"GET", "/999999999999999999999999", "", 400, "invalid_gallery_reference"},
		{"POST", "", `{"gid":42,"token":"other"}`, 409, "download_token_conflict"},
		{"POST", "", `{"gid":0,"token":"token"}`, 400, "invalid_gallery_reference"},
		{"POST", "", `{"gid":1,"token":"token","unknown":true}`, 400, ""},
		{"GET", "/42/file", "", 409, "download_state_conflict"},
		{"DELETE", "/42", "", 409, "download_state_conflict"},
		{"POST", "/42/cancel", "", 200, ""},
		{"POST", "/42/retry", "", 200, ""},
		{"POST", "/42/cancel", "", 200, ""},
		{"DELETE", "/42", "", 204, ""},
		{"GET", "/42", "", 404, "download_not_found"},
	} {
		w := request(tc.method, "/api/collector/downloads"+tc.path, tc.body)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
	if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at, size_bytes) VALUES (7, 'hidden', 'completed', 1, 1, 16)`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "[7].zip"), []byte("retained archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, partial := range []bool{false, true, false} {
		r := httptest.NewRequest("GET", "/api/collector/downloads/7/file", nil)
		r.Header.Set("Authorization", "Bearer browser-secret")
		r.Header.Set("Cookie", "browser-secret=hidden")
		wantStatus, wantBody := 200, "retained archive"
		if partial {
			r.Header.Set("Range", "bytes=0-7")
			wantStatus, wantBody = 206, "retained"
		}
		w := &downloadRecorder{ResponseRecorder: httptest.NewRecorder(), deadline: time.Now().Add(30 * time.Second)}
		handler.ServeHTTP(w, r)
		if !w.deadline.IsZero() || w.Code != wantStatus || w.Body.String() != wantBody ||
			w.Header().Get("Content-Disposition") != `attachment; filename="[7].zip"` ||
			w.Header().Get("Content-Type") != "application/zip" || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("file: deadline %v, status %d, headers %v, body %s", w.deadline, w.Code, w.Header(), w.Body)
		}
		if partial && w.Header().Get("Content-Range") != "bytes 0-7/16" {
			t.Fatalf("range: %v", w.Header())
		}
	}
	if w := request("DELETE", "/api/collector/downloads/7", ""); w.Code != 204 {
		t.Fatalf("delete archive: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, "[7].zip")); !os.IsNotExist(err) {
		t.Fatalf("archive still retained: %v", err)
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-token")
	if err != nil {
		t.Fatal(err)
	}
	handler = NewHandler(&local.App{Logger: logger, Collector: wrong})
	if w := request("GET", "/api/collector/downloads", ""); w.Code != 502 || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("unauthorized: %d %s", w.Code, w.Body)
	}
	handler = NewHandler(&local.App{Logger: logger})
	if w := request("GET", "/api/collector/downloads", ""); w.Code != 503 {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}
