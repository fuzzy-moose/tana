package local_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/httpapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
)

func TestAppLifecycle(t *testing.T) {
	t.Setenv("TANA_DATA_DIR", t.TempDir())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	app, err := local.New(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	handler := httpapi.NewHandler(app)

	body, err := json.Marshal(map[string]string{"name": "Manga", "path": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/libraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create library: %d %s", w.Code, w.Body)
	}
	var created library.Library
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// Process cancellation stops background work; storage remains usable while
	// the HTTP server drains requests, until the application is closed.
	cancel()
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/libraries/"+strconv.FormatInt(created.ID, 10), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("request during shutdown: %d %s", w.Code, w.Body)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := local.New(t.Context(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	w = httptest.NewRecorder()
	httpapi.NewHandler(reopened).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/libraries/"+strconv.FormatInt(created.ID, 10), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("persisted library: %d %s", w.Code, w.Body)
	}
	var restored library.Library
	if err := json.Unmarshal(w.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID != created.ID || restored.Name != created.Name || restored.Path != created.Path {
		t.Fatalf("library changed after restart: %+v", restored)
	}
}

func TestNewRejectsInvalidStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TANA_DATA_DIR", path)
	app, err := local.New(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if app != nil {
		_ = app.Close()
		t.Fatal("returned an application with invalid storage")
	}
	if err == nil {
		t.Fatal("expected storage initialization error")
	}
}

func TestAppServesWebAlongsideAPI(t *testing.T) {
	t.Setenv("TANA_DATA_DIR", t.TempDir())
	webDir := t.TempDir()
	t.Setenv("TANA_WEB_DIR", webDir)
	for name, content := range map[string]string{
		"index.html": "<!doctype html><title>Tana</title>",
		"app.js":     "console.log('tana')",
		"api":        "must not shadow the API",
	} {
		if err := os.WriteFile(filepath.Join(webDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := local.New(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	handler := httpapi.NewHandler(app)

	for _, tt := range []struct {
		method, path, contentType, body string
		status                          int
	}{
		{http.MethodGet, "/", "text/html", "<title>Tana</title>", http.StatusOK},
		{http.MethodHead, "/", "text/html", "", http.StatusOK},
		{http.MethodGet, "/app.js", "text/javascript", "console.log('tana')", http.StatusOK},
		{http.MethodGet, "/api/libraries", "application/json", "", http.StatusOK},
		{http.MethodGet, "/api", "", "", http.StatusNotFound},
		{http.MethodGet, "/api/", "", "", http.StatusNotFound},
		{http.MethodGet, "/api/missing", "", "", http.StatusNotFound},
		{http.MethodPost, "/api/missing", "", "", http.StatusNotFound},
		{http.MethodPut, "/api/libraries", "", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/scans", "", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/healthz", "application/json", `"status":"ok"`, http.StatusOK},
		{http.MethodPost, "/healthz", "", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/", "", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/missing.js", "", "", http.StatusNotFound},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.status || !strings.HasPrefix(w.Header().Get("Content-Type"), tt.contentType) || !strings.Contains(w.Body.String(), tt.body) {
				t.Fatalf("response: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
			}
			if tt.method == http.MethodHead && w.Body.Len() != 0 {
				t.Fatalf("HEAD returned a body: %s", w.Body)
			}
			if tt.status == http.StatusMethodNotAllowed && w.Header().Get("Allow") == "" {
				t.Fatal("missing Allow header")
			}
		})
	}
}

func TestNewRejectsMissingWebBuild(t *testing.T) {
	t.Setenv("TANA_DATA_DIR", t.TempDir())
	t.Setenv("TANA_WEB_DIR", t.TempDir())
	app, err := local.New(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if app != nil {
		_ = app.Close()
		t.Fatal("returned an application without a web build")
	}
	if err == nil || !strings.Contains(err.Error(), "open web UI") {
		t.Fatalf("expected web UI initialization error, got %v", err)
	}
}
