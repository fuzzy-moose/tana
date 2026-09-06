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

	"github.com/fuzzy-moose/tana/internal/collector"
	collectorapi "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	localapi "github.com/fuzzy-moose/tana/internal/local/httpapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/scan"
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
		scans := scan.New(t.Context(), db, libraries, os.DirFS, logger)
		t.Cleanup(scans.Close)
		return localapi.NewHandler(&local.App{
			Logger:    logger,
			Libraries: libraries,
			Scans:     scans,
			Galleries: gallery.NewSQLiteRepository(db),
		})
	}
	collectorHandler := func(logger *slog.Logger) http.Handler {
		return collectorapi.NewHandler(&collector.App{Logger: logger, APIToken: "test-token"})
	}
	for name, newHandler := range map[string]func(*slog.Logger) http.Handler{"local": localHandler, "collector": collectorHandler} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				method, path, body string
				status             int
				origin             string
			}{
				{"GET", "/healthz", `{"status":"ok"}`, 200, ""},
				{"HEAD", "/healthz", `{"status":"ok"}`, 200, ""},
				{"POST", "/healthz", "", 405, ""},
				{"GET", "/missing?secret=hidden", "", 404, ""},
				{"POST", "/healthz", `{"error":"cross_origin_request"}`, 403, "https://untrusted.example"},
			} {
				t.Run(tc.method+tc.path, func(t *testing.T) {
					var logs bytes.Buffer
					handler := newHandler(slog.New(slog.NewJSONHandler(&logs, nil)))
					rr := httptest.NewRecorder()
					req := httptest.NewRequest(tc.method, tc.path, nil)
					if name == "collector" {
						req.Header.Set("Authorization", "Bearer test-token")
					}
					if tc.origin != "" {
						req.Header.Set("Origin", tc.origin)
					}
					handler.ServeHTTP(rr, req)
					if rr.Code != tc.status {
						t.Fatalf("unexpected response: %d %v %s", rr.Code, rr.Header(), rr.Body)
					}
					if tc.body != "" {
						if rr.Header().Get("Content-Type") != "application/json" || rr.Header().Get("Cache-Control") != "no-store" {
							t.Fatalf("unexpected headers: %v", rr.Header())
						}
						// net/http suppresses HEAD bodies on the wire; Recorder does not.
						if tc.method != "HEAD" && strings.TrimSpace(rr.Body.String()) != tc.body {
							t.Fatalf("unexpected body: %s", rr.Body)
						}
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
