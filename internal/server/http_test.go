package server_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	collector "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	local "github.com/fuzzy-moose/tana/internal/local/httpapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func TestServiceRoutes(t *testing.T) {
	for name, newHandler := range map[string]func(*slog.Logger) http.Handler{"local": local.NewHandler, "collector": collector.NewHandler} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				method, path, body string
				status             int
			}{
				{"GET", "/healthz", `{"status":"ok"}`, 200},
				{"HEAD", "/healthz", "", 200},
				{"POST", "/healthz", `{"error":"method_not_allowed"}`, 405},
				{"GET", "/missing?secret=hidden", `{"error":"not_found"}`, 404},
			} {
				t.Run(tc.method+tc.path, func(t *testing.T) {
					var logs bytes.Buffer
					handler := newHandler(slog.New(slog.NewJSONHandler(&logs, nil)))
					rr := httptest.NewRecorder()
					handler.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
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

func TestLoggingRecordsCommittedStatus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		status  int
	}{
		{"empty", func(http.ResponseWriter, *http.Request) {}, 200},
		{"implicit", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }, 200},
		{"duplicate", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(201); w.WriteHeader(500) }, 201},
		{"flush", func(w http.ResponseWriter, _ *http.Request) {
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Error(err)
			}
			w.WriteHeader(500)
		}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := server.Logging(slog.New(slog.NewJSONHandler(&logs, nil)), tc.handler)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
			var record struct {
				Status int `json:"status"`
			}
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record.Status != tc.status || rr.Code != tc.status {
				t.Fatalf("response %d, log %d, want %d", rr.Code, record.Status, tc.status)
			}
		})
	}
}
