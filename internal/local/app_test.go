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
	"testing"

	"github.com/fuzzy-moose/tana/internal/local"
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

	body, err := json.Marshal(map[string]string{"name": "Manga", "path": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/libraries", bytes.NewReader(body)))
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
	app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/libraries/"+created.ID, nil))
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
	reopened.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/libraries/"+created.ID, nil))
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
