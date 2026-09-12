package httpapi

import (
	"context"
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
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type waitingArchiveClient struct{}

func (waitingArchiveClient) GetArchiveURL(ctx context.Context, ref panda.GalleryRef) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestDownloadsAPI(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	logger := slog.New(slog.DiscardHandler)
	service, err := downloads.New(t.Context(), db, dir, waitingArchiveClient{}, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	handler := NewHandler(&collector.App{Logger: logger, Downloads: service, APIToken: "test-token"})
	for range 2 {
		w := apiRequest(handler, "POST", "/api/downloads", `{"gid":42,"token":"gallery-secret"}`)
		if w.Code != http.StatusAccepted || w.Header().Get("Location") != "/api/downloads/42" || strings.Contains(w.Body.String(), "gallery-secret") {
			t.Fatalf("submit: %d %s", w.Code, w.Body)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/downloads", "", 200},
		{"GET", "/api/downloads/42", "", 200},
		{"GET", "/api/downloads/42/file", "", 409},
		{"POST", "/api/downloads/42/retry", "", 409},
		{"DELETE", "/api/downloads/42", "", 409},
		{"POST", "/api/downloads/42/cancel", "", 200},
		{"POST", "/api/downloads/42/retry", "", 200},
		{"POST", "/api/downloads/42/cancel", "", 200},
		{"GET", "/api/downloads/999", "", 404},
		{"GET", "/api/downloads/bad", "", 400},
		{"GET", "/api/downloads?limit=101", "", 400},
		{"GET", "/api/downloads?offset=-1", "", 400},
		{"GET", "/api/downloads?state=unknown", "", 400},
		{"POST", "/api/downloads", `{"gid":42,"token":"other"}`, 409},
		{"POST", "/api/downloads", `{"gid":0,"token":"token"}`, 400},
		{"POST", "/api/downloads", `{"gid":1}`, 400},
		{"POST", "/api/downloads", `{"gid":1,"token":"token","unknown":true}`, 400},
	} {
		w := apiRequest(handler, tc.method, tc.path, tc.body)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
	// A rejected initial token can be corrected by deleting the terminal job.
	if w := apiRequest(handler, "DELETE", "/api/downloads/42", ""); w.Code != 204 {
		t.Fatalf("delete cancelled: %d %s", w.Code, w.Body)
	}
	if w := apiRequest(handler, "POST", "/api/downloads", `{"gid":42,"token":"corrected"}`); w.Code != 202 {
		t.Fatalf("corrected reference: %d %s", w.Code, w.Body)
	}
	if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at) VALUES (7, 'secret', 'completed', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "[7].zip"), []byte("retained archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r := httptest.NewRequest(http.MethodGet, "/api/downloads/7/file", nil)
		r.Header.Set("Authorization", "Bearer test-token")
		w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder(), deadline: time.Now().Add(30 * time.Second)}
		handler.ServeHTTP(w, r)
		if !w.deadline.IsZero() {
			t.Fatal("archive retrieval retained the API write deadline")
		}
		if w.Code != 200 || w.Body.String() != "retained archive" || w.Header().Get("Content-Type") != "application/zip" || w.Header().Get("Content-Disposition") != `attachment; filename="[7].zip"` {
			t.Fatalf("file: %d %s", w.Code, w.Body)
		}
	}
	if w := apiRequest(handler, "DELETE", "/api/downloads/7", ""); w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if w := apiRequest(handler, "GET", "/api/downloads/7/file", ""); w.Code != 404 {
		t.Fatalf("deleted file: %d", w.Code)
	}
}
