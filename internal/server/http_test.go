package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	collector "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	local "github.com/fuzzy-moose/tana/internal/local/httpapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func TestServiceRoutes(t *testing.T) {
	localHandler := func(logger *slog.Logger) http.Handler {
		db, dir, err := storage.Open(context.Background(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		libraries, err := library.New(context.Background(), library.NewSQLiteRepository(db), os.DirFS, dir, logger)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(libraries.Close)
		return local.NewHandler(logger, libraries)
	}
	for name, newHandler := range map[string]func(*slog.Logger) http.Handler{"local": localHandler, "collector": collector.NewHandler} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				method, path, body string
				status             int
				origin             string
			}{
				{"GET", "/healthz", `{"status":"ok"}`, 200, ""},
				{"HEAD", "/healthz", "", 200, ""},
				{"POST", "/healthz", `{"error":"method_not_allowed"}`, 405, ""},
				{"GET", "/missing?secret=hidden", `{"error":"not_found"}`, 404, ""},
				{"POST", "/healthz", `{"error":"cross_origin_request"}`, 403, "https://untrusted.example"},
			} {
				t.Run(tc.method+tc.path, func(t *testing.T) {
					var logs bytes.Buffer
					handler := newHandler(slog.New(slog.NewJSONHandler(&logs, nil)))
					rr := httptest.NewRecorder()
					req := httptest.NewRequest(tc.method, tc.path, nil)
					if tc.origin != "" {
						req.Header.Set("Origin", tc.origin)
					}
					handler.ServeHTTP(rr, req)
					if rr.Code != tc.status || rr.Header().Get("Content-Type") != "application/json" || rr.Header().Get("Cache-Control") != "no-store" {
						t.Fatalf("unexpected response: %d %v %s", rr.Code, rr.Header(), rr.Body)
					}
					// net/http suppresses HEAD bodies on the wire; Recorder does not.
					if tc.method != "HEAD" && strings.TrimSpace(rr.Body.String()) != tc.body {
						t.Fatalf("unexpected body: %s", rr.Body)
					}
					if tc.status == 405 && rr.Header().Get("Allow") != "GET, HEAD" {
						t.Fatal("missing allowed methods")
					}
					var record struct {
						Status int    `json:"status"`
						Method string `json:"method"`
					}
					if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
						t.Fatal(err)
					}
					if record.Status != tc.status || record.Method != tc.method || strings.Contains(logs.String(), "hidden") {
						t.Fatalf("unexpected request log: %s", &logs)
					}
				})
			}
		})
	}
}
