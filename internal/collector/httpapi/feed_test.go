package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

type feedTransport func(*http.Request) (*http.Response, error)

func (f feedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFeedControlProtocol(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
