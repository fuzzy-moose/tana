package feed

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestManualRefreshCoalescesWithScheduledCapture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestDB(t, t.TempDir())
		release := make(chan struct{})
		var requests atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return feedResponse(atomFeed("1/a")), nil
		})}
		s := New(t.Context(), db, Config{URL: testFeedURL, Interval: time.Hour, RetryDelay: time.Minute}, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		for range 5 {
			status, err := s.Refresh(t.Context())
			if err != nil || !status.CaptureActive {
				t.Fatalf("refresh: %+v, %v", status, err)
			}
		}
		close(release)
		synctest.Wait()
		status, err := s.Status(t.Context())
		if err != nil || status.CaptureActive || status.LastCapturedAt == nil || status.ProcessingPending != 0 || requests.Load() != 1 {
			t.Fatalf("completion: %+v requests=%d err=%v", status, requests.Load(), err)
		}
		if _, err := s.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if requests.Load() != 2 {
			t.Fatalf("manual capture waited for schedule: %d", requests.Load())
		}
		if count(t, db, "SELECT count(*) FROM gallery_refs") != 1 {
			t.Fatal("duplicate references")
		}
	})
}

func TestManualRefreshRecoversCaptureErrorAndReportsProcessingFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestDB(t, t.TempDir())
		var requests atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			if requests.Add(1) == 1 {
				return &http.Response{StatusCode: 503, Body: http.NoBody}, nil
			}
			return feedResponse([]byte("broken XML")), nil
		})}
		s := New(t.Context(), db, Config{URL: testFeedURL, Interval: time.Hour, RetryDelay: time.Minute}, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		status, err := s.Status(t.Context())
		if err != nil || status.LastCaptureError == "" || status.LastCaptureErrorAt == nil || status.LastCapturedAt != nil || status.CaptureActive {
			t.Fatalf("capture error: %+v, %v", status, err)
		}
		if _, err := s.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		status, err = s.Status(t.Context())
		if err != nil || status.LastCaptureError != "" || status.LastCaptureErrorAt != nil || status.LastCapturedAt == nil || status.ProcessingPending != 1 || status.ProcessingError == "" || status.Continuity != "unknown" {
			t.Fatalf("processing error: %+v, %v", status, err)
		}
	})
}

func TestStatusRetainsPendingProcessingAndContinuity(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	capture(t, s.store, atomFeed("1/a"), time.Now().Add(-time.Minute))
	capture(t, s.store, atomFeed("2/b"), time.Now())
	status, err := s.Status(t.Context())
	if err != nil || status.ProcessingPending != 2 || status.Continuity != "unknown" {
		t.Fatalf("pending: %+v, %v", status, err)
	}
	s.processPending(t.Context())
	status, err = s.Status(t.Context())
	if err != nil || status.ProcessingPending != 0 || status.Continuity != "possible_gap" || status.PossibleGaps != 1 {
		t.Fatalf("processed: %+v, %v", status, err)
	}
}
