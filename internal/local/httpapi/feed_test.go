package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/feed"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestLocalRawFeedCaptures(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw := []byte("<?xml version=\"1.0\"?>\r\n<feed xmlns=\"http://www.w3.org/2005/Atom\"><title>bad\x1ddata</feed>\r\n")
	_, parseErr := panda.ParseFeed(bytes.NewReader(raw))
	if parseErr == nil {
		t.Fatal("expected malformed XML failure")
	}
	if _, err := db.Exec(`INSERT INTO raw_feeds (id, captured_at, feed_url, body, processed_at, last_error) VALUES
		(338, 1000, 'https://panda.test/feed', ?, 3000, NULL),
		(339, 2000, 'https://panda.test/feed', ?, NULL, ?),
		(340, 2000, 'https://panda.test/feed', ?, NULL, NULL)`, []byte("<feed/>"), raw, parseErr.Error(), []byte("<feed/>")); err != nil {
		t.Fatal(err)
	}
	// Keep the retained processing states stable while exercising both HTTP layers.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	logger := slog.New(slog.DiscardHandler)
	service := feed.New(ctx, db, feed.Config{Interval: time.Hour}, nil, logger)
	service.Close()
	collectorHandler := collectorhttp.NewHandler(&collector.App{Feeds: service, Logger: logger, APIToken: "server-secret"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer server-secret" {
			t.Error("collector request did not isolate browser credentials")
		}
		w.Header().Set("Set-Cookie", "collector-secret=hidden")
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Collector: client, Logger: logger})
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/collector/feed/captures"+path, nil)
		r.Header.Set("Cookie", "browser-secret=hidden")
		r.Header.Set("Authorization", "Bearer browser-secret")
		w := &downloadRecorder{ResponseRecorder: httptest.NewRecorder(), deadline: time.Now().Add(30 * time.Second)}
		handler.ServeHTTP(w, r)
		if strings.HasSuffix(path, "/file") && w.Code == http.StatusOK && !w.deadline.IsZero() {
			t.Fatal("raw feed download retained the API write deadline")
		}
		if w.Header().Get("Set-Cookie") != "" {
			t.Fatal("collector cookie exposed")
		}
		return w.ResponseRecorder
	}
	for _, tc := range []struct {
		query   string
		ids     []int64
		hasMore bool
	}{
		{"", []int64{340, 339}, false},
		{"?limit=1", []int64{340}, true},
		{"?limit=1&offset=1", []int64{339}, false},
		{"?limit=1&offset=2", []int64{}, false},
		{"?failed_only=true&limit=1", []int64{339}, false},
		{"?offset=3", []int64{}, false},
		{"?failed_only=true&offset=1", []int64{}, false},
	} {
		w := request("GET", tc.query)
		var result collectorapi.FeedCaptureList
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || result.Captures == nil {
			t.Fatalf("list %s: %d %s, %v", tc.query, w.Code, w.Body, err)
		}
		ids := []int64{}
		for _, capture := range result.Captures {
			ids = append(ids, capture.ID)
			wantState := map[int64]string{339: "failed", 340: "pending"}[capture.ID]
			if capture.State != wantState {
				t.Fatalf("capture %d state = %s", capture.ID, capture.State)
			}
			if capture.ID == 339 && (capture.Error != parseErr.Error() || capture.SizeBytes != int64(len(raw)) || !capture.CapturedAt.Equal(time.UnixMilli(2000))) {
				t.Fatalf("failed capture details: %+v", capture)
			}
		}
		if !reflect.DeepEqual(ids, tc.ids) || result.HasMore != tc.hasMore {
			t.Fatalf("list %s: %+v", tc.query, result)
		}
	}
	for _, id := range []int{339, 340, 339} {
		w := request("GET", fmt.Sprintf("/%d/file", id))
		want := raw
		if id != 339 {
			want = []byte("<feed/>")
		}
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
			t.Fatalf("raw capture %d changed: %d %q", id, w.Code, w.Body.Bytes())
		}
		if w.Header().Get("Content-Disposition") != fmt.Sprintf(`attachment; filename="panda-feed-%d.xml"`, id) ||
			w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Length") != strconv.Itoa(len(want)) ||
			w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("download headers: %v", w.Header())
		}
	}
	w := request("HEAD", "/339/file")
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Length") != strconv.Itoa(len(raw)) {
		t.Fatalf("HEAD: %d %v %s", w.Code, w.Header(), w.Body)
	}
	for _, suffix := range []string{"/338/file", "/999/file", "/bad/file", "/0/file", "/-1/file", "/99999999999999999999/file"} {
		if w := request("GET", suffix); w.Code != http.StatusNotFound {
			t.Fatalf("missing capture %s: %d %s", suffix, w.Code, w.Body)
		}
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=-1", "?offset=bad", "?failed_only=bad"} {
		if w := request("GET", query); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %s: %d %s", query, w.Code, w.Body)
		}
	}
	for _, suffix := range []string{"", "/339/file"} {
		w := httptest.NewRecorder()
		collectorHandler.ServeHTTP(w, httptest.NewRequest("GET", "/api/feed/captures"+suffix, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated capture access: %d", w.Code)
		}
	}
}

func TestRawFeedCapturesWithoutCollector(t *testing.T) {
	for _, handler := range []http.Handler{HandleCollectorFeedCaptures(nil), HandleCollectorFeedCaptureFile(nil)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
		}
	}
}
