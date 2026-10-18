package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/server"
)

func TestLibraryInternalErrorUsesRequestLog(t *testing.T) {
	var logs bytes.Buffer
	h := server.HTTPContextMiddleware(server.LoggingMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeLibraryError(w, r, errors.New("database failed"))
	})))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/libraries", nil))
	if w.Code != 500 || strings.TrimSpace(w.Body.String()) != `{"error":"internal_error"}` {
		t.Fatalf("unexpected internal error response: %d %s", w.Code, w.Body)
	}
	var record struct {
		Level, Error string
		Status       int
	}
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Level != "ERROR" || record.Status != 500 || record.Error != "database failed" {
		t.Fatalf("unexpected request error log: %s", &logs)
	}
}

func TestLibraryJSONMediaTypes(t *testing.T) {
	h := testHandler(t)
	created := decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, "Original", t.TempDir()), 201))
	for _, endpoint := range []struct{ method, path, body string }{
		{"POST", "/api/libraries", registrationJSON(t, "New", t.TempDir())},
		{"PATCH", "/api/libraries/" + strconv.FormatInt(created.ID, 10), `{"name":"Changed"}`},
	} {
		for _, contentType := range []string{"", "text/plain"} {
			t.Run(endpoint.method+contentType, func(t *testing.T) {
				r := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
				if contentType != "" {
					r.Header.Set("Content-Type", contentType)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 415 || strings.TrimSpace(w.Body.String()) != `{"error":"unsupported_media_type"}` {
					t.Fatalf("unexpected media type response: %d %s", w.Code, w.Body)
				}
			})
		}
	}
}
