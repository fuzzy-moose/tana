package httpapi

import (
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/scan"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	return testHandlerWithScanFS(t, os.DirFS)
}

func testHandlerWithScanFS(t *testing.T, dirFS func(string) fs.FS) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, dir, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	libraries, err := library.New(t.Context(), library.NewSQLiteRepository(db), os.DirFS, dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(libraries.Close)
	scans := scan.New(t.Context(), db, libraries, dirFS, logger)
	t.Cleanup(scans.Close)
	return NewHandler(&local.App{
		Logger:    logger,
		Libraries: libraries,
		Scans:     scans,
		Galleries: gallery.NewSQLiteRepository(db),
	})
}

func request(t *testing.T, handler http.Handler, method, path, body string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != wantStatus {
		t.Fatalf("%s %s: got %d %s, want %d", method, path, w.Code, w.Body, wantStatus)
	}
	if w.Code == http.StatusMethodNotAllowed {
		if w.Header().Get("Allow") == "" {
			t.Fatal("missing Allow header")
		}
		return w
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing cache policy")
	}
	return w
}

func decodeLibrary(t *testing.T, w *httptest.ResponseRecorder) library.Library {
	t.Helper()
	var result library.Library
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func registrationJSON(t *testing.T, name, path string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"name": name, "path": path})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestLibraryAPILifecycle(t *testing.T) {
	h := testHandler(t)
	if body := request(t, h, "GET", "/api/libraries", "", 200).Body.String(); strings.TrimSpace(body) != "[]" {
		t.Fatalf("empty list: %s", body)
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "nas")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "comic.cbz")
	if err := os.WriteFile(file, []byte("leave comics untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := request(t, h, "POST", "/api/libraries", registrationJSON(t, "  Manga  ", root), 201)
	created := decodeLibrary(t, w)
	if created.ID == 0 || created.Name != "Manga" || created.Path != root || created.Availability != "available" || created.LastCheckedAt == nil {
		t.Fatalf("invalid representation: %+v", created)
	}
	path := "/api/libraries/" + strconv.FormatInt(created.ID, 10)
	if w.Header().Get("Location") != path {
		t.Fatal("missing resource location")
	}
	request(t, h, "POST", "/api/libraries", registrationJSON(t, "Duplicate", root), 409)
	request(t, h, "POST", "/api/libraries", registrationJSON(t, "Manga", t.TempDir()), 201)
	var all []library.Library
	if err := json.Unmarshal(request(t, h, "GET", "/api/libraries", "", 200).Body.Bytes(), &all); err != nil || len(all) != 2 {
		t.Fatalf("multiple libraries: %+v, %v", all, err)
	}
	if got := decodeLibrary(t, request(t, h, "GET", path, "", 200)); got.ID != created.ID {
		t.Fatal("wrong library returned")
	}
	renamed := decodeLibrary(t, request(t, h, "PATCH", path, `{"name":"  Comics  "}`, 200))
	if renamed.Name != "Comics" || renamed.ID != created.ID || renamed.Path != created.Path {
		t.Fatalf("invalid rename: %+v", renamed)
	}
	// Simulate an unavailable NAS without deleting any files.
	backup := filepath.Join(parent, "disconnected")
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	request(t, h, "POST", path+"/availability-check", "", 202)
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := decodeLibrary(t, request(t, h, "GET", path, "", 200))
		if got.Availability == "unavailable" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("asynchronous check did not report outage")
		}
		time.Sleep(time.Millisecond)
	}
	request(t, h, "PATCH", path, `{"name":"Offline comics"}`, 200)
	request(t, h, "DELETE", path, "", 204)
	request(t, h, "DELETE", path, "", 204)
	request(t, h, "GET", path, "", 404)
	request(t, h, "POST", path+"/availability-check", "", 404)
	content, err := os.ReadFile(filepath.Join(backup, "comic.cbz"))
	if err != nil || string(content) != "leave comics untouched" {
		t.Fatalf("removal changed files: %q, %v", content, err)
	}
	if err := os.Rename(backup, root); err != nil {
		t.Fatal(err)
	}
	readded := decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, "Fresh", root), 201))
	if readded.ID == created.ID || readded.Name != "Fresh" {
		t.Fatalf("re-add restored removed library: %+v", readded)
	}
}

func TestLibraryAPIValidation(t *testing.T) {
	h := testHandler(t)
	for _, tc := range []struct {
		method, path, body, code string
		status                   int
	}{
		{"POST", "/api/libraries", "{", "invalid_json", 400},
		{"POST", "/api/libraries", `{}`, "invalid_name", 400},
		{"POST", "/api/libraries", `{"name":"x","path":"relative"}`, "invalid_path", 400},
		{"POST", "/api/libraries", `{"name":"x","extra":1}`, "invalid_json", 400},
		{"POST", "/api/libraries", `{"name":"` + strings.Repeat("x", 17000) + `"}`, "invalid_json", 400},
		{"GET", "/api/libraries/missing", "", "not_found", 404},
	} {
		t.Run(tc.method+tc.path+tc.code, func(t *testing.T) {
			w := request(t, h, tc.method, tc.path, tc.body, tc.status)
			var result map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["error"] != tc.code {
				t.Fatalf("unexpected error: %s, %v", w.Body, err)
			}
		})
	}
	request(t, h, "PUT", "/api/libraries", "", 405)
	request(t, h, "POST", "/api/libraries", registrationJSON(t, "Missing", filepath.Join(t.TempDir(), "missing")), 422)
}
