package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/api/libraries"},
		{"PATCH", "/api/libraries/" + created.ID},
	} {
		for _, tc := range []struct {
			contentTypes []string
			valid        bool
		}{
			{nil, false},
			{[]string{"text/plain"}, false},
			{[]string{"application/x-www-form-urlencoded"}, false},
			{[]string{"application/problem+json"}, false},
			{[]string{"application/json; charset"}, false},
			{[]string{"application/json", "text/plain"}, false},
			{[]string{"application/json"}, true},
			{[]string{"Application/JSON; charset=utf-8"}, true},
		} {
			t.Run(endpoint.method+strings.Join(tc.contentTypes, ","), func(t *testing.T) {
				body := `{"name":"Changed"}`
				want := 200
				if endpoint.method == "POST" {
					body = registrationJSON(t, "New", t.TempDir())
					want = 201
				}
				if !tc.valid {
					want = 415
				}
				r := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(body))
				for _, value := range tc.contentTypes {
					r.Header.Add("Content-Type", value)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != want {
					t.Fatalf("status=%d body=%s, want %d", w.Code, w.Body, want)
				}
				if !tc.valid && strings.TrimSpace(w.Body.String()) != `{"error":"unsupported_media_type"}` {
					t.Fatalf("unexpected media type error: %s", w.Body)
				}
			})
		}
	}
}

func TestLibraryCrossOriginProtection(t *testing.T) {
	h := testHandler(t)
	created := decodeLibrary(t, request(t, h, "POST", "/api/libraries", registrationJSON(t, "Original", t.TempDir()), 201))
	path := "/api/libraries/" + created.ID
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/api/libraries"}, {"PATCH", path}, {"DELETE", path}, {"POST", path + "/availability-check"},
	} {
		for _, headers := range []map[string]string{
			{"Origin": "https://untrusted.example"},
			{"Origin": "null"},
			{"Sec-Fetch-Site": "cross-site"},
			{"Sec-Fetch-Site": "same-site", "Origin": "http://example.com"},
		} {
			r := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`{"name":"Changed"}`))
			r.Header.Set("Content-Type", "application/json")
			for name, value := range headers {
				r.Header.Set(name, value)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 || strings.TrimSpace(w.Body.String()) != `{"error":"cross_origin_request"}` {
				t.Fatalf("%s %s %v: %d %s", endpoint.method, endpoint.path, headers, w.Code, w.Body)
			}
		}
	}
	if got := decodeLibrary(t, request(t, h, "GET", path, "", 200)); got.Name != "Original" {
		t.Fatal("rejected request changed the library")
	}
	for _, headers := range []map[string]string{
		{}, {"Origin": "http://example.com"}, {"Sec-Fetch-Site": "same-origin"}, {"Sec-Fetch-Site": "none"},
	} {
		r := httptest.NewRequest("PATCH", path, strings.NewReader(`{"name":"Allowed"}`))
		r.Header.Set("Content-Type", "application/json")
		for name, value := range headers {
			r.Header.Set(name, value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("allowed caller %v: %d %s", headers, w.Code, w.Body)
		}
	}
	// A bodyless operation does not acquire a JSON Content-Type requirement.
	r := httptest.NewRequest("POST", path+"/availability-check", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 202 {
		t.Fatalf("bodyless check: %d %s", w.Code, w.Body)
	}
}
