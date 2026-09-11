package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
)

type importDeadlineRecorder struct {
	*httptest.ResponseRecorder
	read, write time.Time
}

func (w *importDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	w.read = deadline
	return nil
}

func (w *importDeadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.write = deadline
	return nil
}

type interruptedImportReader struct{}

func (interruptedImportReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestCollectorReferenceImportProxy(t *testing.T) {
	const filename = "日本 + #&.jsonl"
	const input = "{\"gid\":42,\"token\":\"reference-secret\"}\n"
	var requests, accepted atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer server-secret" {
			t.Error("incorrect collector credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "upstream-secret=hidden")
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/reference-imports":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			if string(body) != input || r.URL.Query().Get("filename") != filename {
				t.Errorf("upload = %s %q", r.URL, body)
			}
			accepted.Add(1)
			w.WriteHeader(202)
			_, _ = io.WriteString(w, `{"id":"accepted","status":"processing","size_bytes":39}`)
		case r.Method == "GET" && r.URL.Path == "/api/reference-imports":
			if r.URL.Query().Get("limit") != "25" || r.URL.Query().Get("offset") != "2" {
				t.Errorf("pagination = %s", r.URL)
			}
			_, _ = io.WriteString(w, `{"imports":[]}`)
		case r.Method == "POST" && r.URL.Path == "/api/reference-imports/accepted/cancel":
			_, _ = io.WriteString(w, `{"id":"accepted","status":"cancelled"}`)
		case r.Method == "POST" && r.URL.Path == "/api/reference-imports/accepted/retry":
			w.WriteHeader(409)
			_, _ = io.WriteString(w, `{"error":"import_state_conflict"}`)
		default:
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"error":"import_not_found"}`)
		}
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Collector: client})
	request := func(method, path string, body io.Reader, length int64) *importDeadlineRecorder {
		r := httptest.NewRequest(method, "/api/collector/reference-imports"+path, body)
		r.ContentLength = length
		r.Header.Set("Content-Type", "application/x-ndjson")
		r.Header.Set("Cookie", "browser-secret=hidden")
		r.Header.Set("Authorization", "Bearer browser-secret")
		w := &importDeadlineRecorder{ResponseRecorder: httptest.NewRecorder(), read: time.Now(), write: time.Now()}
		handler.ServeHTTP(w, r)
		if w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("collector secret exposed: %v %s", w.Header(), w.Body)
		}
		return w
	}
	query := "?filename=" + url.QueryEscape(filename)
	w := request("POST", query, strings.NewReader(input), int64(len(input)))
	if w.Code != 202 || w.Header().Get("Location") != "/api/collector/reference-imports/accepted" || !w.read.IsZero() || !w.write.IsZero() {
		t.Fatalf("submit: %d %v %s, deadlines %v/%v", w.Code, w.Header(), w.Body, w.read, w.write)
	}
	before := requests.Load()
	w = request("POST", query, strings.NewReader(""), collectorapi.MaxReferenceImportBytes+1)
	if w.Code != 413 || requests.Load() != before {
		t.Fatalf("oversize upload forwarded: %d %s", w.Code, w.Body)
	}
	w = request("POST", query, interruptedImportReader{}, -1)
	if w.Code != 400 || accepted.Load() != 1 {
		t.Fatalf("interrupted upload: %d %s; accepted %d", w.Code, w.Body, accepted.Load())
	}
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{"GET", "?limit=25&offset=2", "", 200},
		{"GET", "?limit=101", "invalid_pagination", 400},
		{"GET", "?offset=-1", "invalid_pagination", 400},
		{"POST", "", "invalid_filename", 400},
		{"GET", "/missing", "import_not_found", 404},
		{"POST", "/accepted/cancel", "cancelled", 200},
		{"POST", "/accepted/retry", "import_state_conflict", 409},
	} {
		w := request(tc.method, tc.path, nil, 0)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
	handler = NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler)})
	w = request("GET", "", nil, 0)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured collector: %d %s", w.Code, w.Body)
	}
}
