package panda_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
	"golang.org/x/time/rate"
)

func newTestRateLimiter(t *testing.T) *rate.Limiter {
	t.Helper()
	cfg, err := panda.LoadConfig(func(key string) string {
		if key == "PANDA_API_URL" {
			return testAPIURL
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := panda.NewRateLimiter(cfg.RateInterval, cfg.RateBurst)
	if err != nil {
		t.Fatal(err)
	}
	return limiter
}

func TestRateLimitedTransportCountsRequestsAcrossPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		galleries := make([]panda.GalleryRef, 25)
		metadata := make([]panda.Metadata, len(galleries))
		for i := range galleries {
			galleries[i] = panda.GalleryRef{ID: int64(i + 1), Token: "token"}
			metadata[i] = panda.Metadata{ID: galleries[i].ID, Token: galleries[i].Token}
		}
		body, err := json.Marshal(map[string]any{"gmetadata": metadata})
		if err != nil {
			t.Fatal(err)
		}
		var requests atomic.Int32
		transport := panda.RateLimitedTransport(newTestRateLimiter(t), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			defer r.Body.Close()
			requests.Add(1)
			return jsonResponse(string(body)), nil
		}))
		client := newTestClient(t, &http.Client{Transport: transport})
		start := time.Now()
		// Validation failures send no request and must not consume a token.
		if _, err := client.GetMetadata(t.Context(), nil); err == nil {
			t.Fatal("expected empty batch to fail validation")
		}
		// One page needs four requests, each containing 25 galleries.
		for range 4 {
			if _, err := client.GetMetadata(t.Context(), galleries); err != nil {
				t.Fatal(err)
			}
		}
		if elapsed := time.Since(start); elapsed != 0 || requests.Load() != 4 {
			t.Fatalf("first page: %d requests after %v, want 4 immediately", requests.Load(), elapsed)
		}

		// The next page uses the same transport and must wait before sending.
		done := make(chan error, 1)
		go func() {
			_, err := client.GetMetadata(t.Context(), galleries)
			done <- err
		}()
		synctest.Wait()
		if requests.Load() != 4 {
			t.Fatalf("sent request before limiter allowed it: %d requests", requests.Load())
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed != 2500*time.Millisecond || requests.Load() != 5 {
			t.Fatalf("next page: %d requests after %v, want 5 after 2.5s", requests.Load(), elapsed)
		}
	})
}

func TestRateLimitedTransportCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := newTestRateLimiter(t)
		// Arrange an exhausted bucket so the transport must wait.
		if !limiter.AllowN(time.Now(), 4) {
			t.Fatal("could not exhaust initial burst")
		}
		called := false
		transport := panda.RateLimitedTransport(limiter, roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return jsonResponse(`{}`), nil
		}))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		body := &trackedBody{Reader: strings.NewReader(`{}`)}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com", body)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := transport.RoundTrip(req)
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
		if called || !body.closed {
			t.Fatalf("canceled request: sent = %v, body closed = %v", called, body.closed)
		}
	})
}
