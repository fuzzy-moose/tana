package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
	"github.com/fuzzy-moose/tana/internal/server"
)

type feedTransport func(*http.Request) (*http.Response, error)

func (f feedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFeedCaptureFileClearsWriteDeadline(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	const raw = "<feed/>\r\n"
	if _, err := db.Exec(t.Context(), `INSERT INTO raw_feeds (id, captured_at, feed_url, body) VALUES (42, to_timestamp(1), 'https://panda.test/feed', $1)`, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	service := feed.New(ctx, db, feed.Config{Interval: time.Hour}, nil, slog.New(slog.DiscardHandler))
	service.Close()
	handler := server.HTTPContextMiddleware(HandleFeedCaptureFile(service))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRequest(method, "/api/feed/captures/42/file", nil)
		r.SetPathValue("id", "42")
		w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder(), deadline: time.Now().Add(30 * time.Second)}
		handler.ServeHTTP(w, r)
		if !w.deadline.IsZero() {
			t.Fatal("raw feed download retained the API write deadline")
		}
		want := raw
		if method == http.MethodHead {
			want = ""
		}
		if w.Code != http.StatusOK || w.Body.String() != want {
			t.Fatalf("%s capture: %d %q", method, w.Code, w.Body.String())
		}
	}
}

func TestFeedControlProtocol(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	// Keep capture running so both reads and repeated commands observe one run.
	upstream := &http.Client{Transport: feedTransport(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	s := feed.New(t.Context(), db, feed.Config{URL: "https://panda.test/feed", Interval: time.Hour, RetryDelay: time.Minute}, upstream, slog.New(slog.DiscardHandler))
	defer s.Close()
	mux := http.NewServeMux()
	mux.Handle("GET /api/feed/status", HandleFeed(s))
	mux.Handle("POST /api/feed/refresh", HandleFeed(s))
	handler := server.HTTPContextMiddleware(mux)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	client, err := collectorapi.NewClient(httpServer.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.FeedStatus(t.Context())
	if err != nil || status.LastCapturedAt != nil || status.Continuity != "unknown" {
		t.Fatalf("status: %+v, %v", status, err)
	}
	for range 2 {
		status, err = client.RefreshFeed(t.Context())
		if err != nil || !status.CaptureActive {
			t.Fatalf("refresh: %+v, %v", status, err)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/feed/refresh", strings.NewReader(`{"unknown":true}`))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid request: %d %s", w.Code, w.Body)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.RefreshFeed(ctx); err == nil {
		t.Fatal("cancelled request accepted")
	}
}

func TestFeedCaptureListTotalIgnoresPagination(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	if _, err := db.Exec(t.Context(), `INSERT INTO raw_feeds (id, captured_at, feed_url, body, last_error) VALUES
		(1, to_timestamp(1), 'https://panda.test/feed', '\x01', NULL),
		(2, to_timestamp(2), 'https://panda.test/feed', '\x02', 'bad xml'),
		(3, to_timestamp(3), 'https://panda.test/feed', '\x03', NULL)`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	logger := slog.New(slog.DiscardHandler)
	service := feed.New(ctx, db, feed.Config{Interval: time.Hour}, nil, logger)
	service.Close()
	handler := NewHandler(&collector.App{Logger: logger, Feeds: service, APIToken: "test-token"})
	for _, tc := range []struct {
		query string
		ids   []int64
		total int64
	}{
		{"", []int64{3, 2, 1}, 3},
		{"?limit=2", []int64{3, 2}, 3},
		{"?failed_only=true", []int64{2}, 1},
		{"?limit=2&offset=10", []int64{}, 3},
		{"?failed_only=true&offset=5", []int64{}, 1},
	} {
		w := apiRequest(handler, http.MethodGet, "/api/feed/captures"+tc.query, "")
		var result collectorapi.FeedCaptureList
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || result.Captures == nil {
			t.Fatalf("list %s: %d %s, %v", tc.query, w.Code, w.Body, err)
		}
		ids := []int64{}
		for _, capture := range result.Captures {
			ids = append(ids, capture.ID)
		}
		if !slices.Equal(ids, tc.ids) || result.Total != tc.total {
			t.Fatalf("list %s: ids %v total %d", tc.query, ids, result.Total)
		}
	}
}
