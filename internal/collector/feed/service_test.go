package feed

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
)

const testFeedURL = "https://panda.example.test/feed?mode=atom"

func openTestDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func atomFeed(refs ...string) []byte {
	var body strings.Builder
	body.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom">`)
	for _, ref := range refs {
		fmt.Fprintf(&body, `<entry><link href="https://panda.example.test/g/%s/" /></entry>`, ref)
	}
	body.WriteString(`</feed>`)
	return []byte(body.String())
}

func capture(t *testing.T, s *store, body []byte, at time.Time) int64 {
	t.Helper()
	id, err := s.capture(t.Context(), testFeedURL, body, at)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func checkStatus(t *testing.T, db *sql.DB, previous, current int64, want string) {
	t.Helper()
	var status string
	var created, checked int64
	if err := db.QueryRow(`SELECT status, created_at, checked_at FROM feed_continuity_checks
		WHERE previous_capture_id = ? AND current_capture_id = ?`, previous, current).Scan(&status, &created, &checked); err != nil {
		t.Fatal(err)
	}
	if status != want || created == 0 || checked < created {
		t.Fatalf("check %d -> %d: %s, created %d checked %d; want %s", previous, current, status, created, checked, want)
	}
}

func TestProcessingRetainsCapturesAndContinuesPastBadFeed(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	var logs bytes.Buffer
	s := &Service{store: newStore(db), logger: slog.New(slog.NewTextHandler(&logs, nil))}
	bodies := [][]byte{
		atomFeed("1/a", "1/another-token", "2/b"), atomFeed("2/b", "3/c"),
		[]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry/>`),
		atomFeed("4/d"), atomFeed(), atomFeed("5/e"), atomFeed("5/changed-token"), atomFeed("6/f"),
	}
	var ids []int64
	for i, body := range bodies {
		ids = append(ids, capture(t, s.store, body, time.Now().Add(time.Duration(i-10)*time.Minute)))
	}
	if n := count(t, db, "SELECT count(*) FROM feed_continuity_checks WHERE status = 'unknown'"); n != len(ids)-1 {
		t.Fatalf("pending comparisons = %d", n)
	}
	s.processPending(t.Context())
	if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL"); n != 7 {
		t.Fatalf("processed = %d, want 7", n)
	}
	if n := count(t, db, "SELECT count(*) FROM gallery_refs"); n != 6 {
		t.Fatalf("unique references = %d, want 6", n)
	}
	for i, id := range ids {
		body, err := s.store.q.CaptureBody(t.Context(), id)
		if err != nil || !bytes.Equal(body, bodies[i]) {
			t.Fatalf("raw capture %d changed: %q, %v", id, body, err)
		}
	}
	for i, want := range []string{"overlap", "unknown", "possible_gap", "unknown", "possible_gap", "overlap", "possible_gap"} {
		checkStatus(t, db, ids[i], ids[i+1], want)
	}
	var lastError string
	var attempted int64
	if err := db.QueryRow("SELECT last_error, last_attempt_at FROM raw_feeds WHERE id = ?", ids[2]).Scan(&lastError, &attempted); err != nil || lastError == "" || attempted == 0 {
		t.Fatalf("missing failure details: %q, %d, %v", lastError, attempted, err)
	}
	s.processPending(t.Context())
	if n := strings.Count(logs.String(), "msg=feed_processing_failed"); n != 2 {
		t.Fatalf("failed capture attempted %d times, want once per run", n)
	}
	if n := strings.Count(logs.String(), "level=WARN msg=feed_possible_gap"); n != 3 {
		t.Fatalf("gap warnings = %d; logs: %s", n, &logs)
	}
	if !strings.Contains(logs.String(), fmt.Sprintf("previous_capture_id=%d current_capture_id=%d", ids[6], ids[7])) {
		t.Fatalf("warning missing capture references: %s", &logs)
	}
}

func TestProcessingRollsBackAndDetectsKnownGalleryOnRetry(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	first := capture(t, s.store, atomFeed("1/a"), time.Now().Add(-3*time.Minute))
	failed := capture(t, s.store, atomFeed("2/b", "3/c"), time.Now().Add(-2*time.Minute))
	last := capture(t, s.store, atomFeed("3/c"), time.Now().Add(-time.Minute))
	_, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_completion BEFORE UPDATE OF processed_at ON raw_feeds
		WHEN NEW.id = %d BEGIN SELECT RAISE(FAIL, 'simulated storage failure'); END`, failed))
	if err != nil {
		t.Fatal(err)
	}
	s.processPending(t.Context())
	if n := count(t, db, "SELECT count(*) FROM gallery_refs WHERE gallery_id = 2"); n != 0 {
		t.Fatal("reference committed without completion")
	}
	if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL"); n != 2 {
		t.Fatalf("processed %d, want 2 despite failure", n)
	}
	checkStatus(t, db, first, failed, "unknown")
	checkStatus(t, db, failed, last, "possible_gap")
	if _, err := db.Exec("DROP TRIGGER reject_completion"); err != nil {
		t.Fatal(err)
	}
	s.processPending(t.Context())
	checkStatus(t, db, first, failed, "overlap")
	checkStatus(t, db, failed, last, "possible_gap")
	if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL AND last_error IS NULL"); n != 3 {
		t.Fatalf("successful captures after retry = %d", n)
	}
}

func TestOverlapUsesExistingIDsAndIgnoresDuplicatesWithinCapture(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	at := time.Now().Add(-time.Hour)
	first := capture(t, s.store, atomFeed("1/a"), at)
	second := capture(t, s.store, atomFeed("2/b", "2/another-token"), at.Add(time.Minute))
	third := capture(t, s.store, atomFeed("1/changed-token"), at.Add(2*time.Minute))
	s.processPending(t.Context())
	checkStatus(t, db, first, second, "possible_gap")
	checkStatus(t, db, second, third, "overlap")
	if n := count(t, db, "SELECT count(*) FROM gallery_refs"); n != 2 {
		t.Fatalf("unique galleries = %d, want 2", n)
	}
}

func TestContinuityUsesCaptureTimeAndIDForTies(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	at := time.Now().Add(-time.Hour)
	last := capture(t, s.store, atomFeed("3/c"), at.Add(time.Minute))
	first := capture(t, s.store, atomFeed("1/a"), at)
	middle := capture(t, s.store, atomFeed("1/a", "3/c"), at)
	ids, err := s.store.q.PendingCaptures(t.Context())
	if err != nil || !reflect.DeepEqual(ids, []int64{first, middle, last}) {
		t.Fatalf("pending order = %v, %v", ids, err)
	}
	s.processPending(t.Context())
	checkStatus(t, db, first, middle, "overlap")
	checkStatus(t, db, middle, last, "overlap")
	if n := count(t, db, "SELECT count(*) FROM feed_continuity_checks"); n != 2 {
		t.Fatalf("comparisons = %d, want 2", n)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func feedResponse(body []byte) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestRestartUsesStoredScheduleAndProcessesPendingCaptures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		db := openTestDB(t, dir)
		capture(t, newStore(db), atomFeed("1/a"), time.Now().Add(-10*time.Minute))
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openTestDB(t, dir)
		var requests atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests.Add(1)
			if req.Method != http.MethodGet || req.URL.String() != testFeedURL {
				t.Errorf("unexpected request: %s %s", req.Method, req.URL)
			}
			return feedResponse(atomFeed("1/a", "2/b")), nil
		})}
		cfg := Config{URL: testFeedURL, Interval: 30 * time.Minute, RetryDelay: time.Minute}
		s := New(t.Context(), db, cfg, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		if requests.Load() != 0 || count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL") != 1 {
			t.Fatal("startup must process pending data without fetching early")
		}
		time.Sleep(20*time.Minute - time.Second)
		synctest.Wait()
		if requests.Load() != 0 {
			t.Fatal("downloaded before stored capture expired")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if requests.Load() != 1 || count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL") != 2 {
			t.Fatal("due capture was not downloaded and processed")
		}
		s.Close()
		s = New(t.Context(), db, cfg, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		if requests.Load() != 1 {
			t.Fatal("restart triggered a repeated download")
		}
		time.Sleep(30 * time.Minute)
		synctest.Wait()
		if requests.Load() != 2 {
			t.Fatalf("requests = %d, want 2", requests.Load())
		}
	})
}

func TestEmptyDatabaseFetchesImmediatelyAndFailuresWaitForRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestDB(t, t.TempDir())
		var requests atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			if requests.Load() == 1 {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			}
			return feedResponse(atomFeed("1/a")), nil
		})}
		s := New(t.Context(), db, Config{URL: testFeedURL, Interval: time.Hour, RetryDelay: 2 * time.Minute}, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		if requests.Load() != 1 || count(t, db, "SELECT count(*) FROM raw_feeds") != 0 {
			t.Fatal("empty database should fetch immediately, without saving HTTP errors")
		}
		time.Sleep(2*time.Minute - time.Second)
		synctest.Wait()
		if requests.Load() != 1 {
			t.Fatal("retried before configured delay")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if requests.Load() != 2 || count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL") != 1 {
			t.Fatal("failed download did not recover on retry")
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if requests.Load() != 2 {
			t.Fatal("successful capture did not restore regular interval")
		}
	})
}

type blockingHandler struct {
	slog.Handler
	release chan struct{}
	once    sync.Once
}

func (h *blockingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "feed_processed" {
		h.once.Do(func() { <-h.release })
	}
	return h.Handler.Handle(ctx, r)
}

func TestDownloadsContinueWhileProcessorIsBusyAndTriggersAreRetained(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestDB(t, t.TempDir())
		var requests atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return feedResponse(atomFeed(fmt.Sprintf("%d/token", requests.Load()))), nil
		})}
		handler := &blockingHandler{Handler: slog.NewTextHandler(io.Discard, nil), release: make(chan struct{})}
		s := New(t.Context(), db, Config{URL: testFeedURL, Interval: time.Minute, RetryDelay: time.Minute}, client, slog.New(handler))
		defer s.Close()
		defer func() {
			select {
			case <-handler.release:
			default:
				close(handler.release)
			}
		}()
		synctest.Wait()
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if requests.Load() != 3 || count(t, db, "SELECT count(*) FROM raw_feeds") != 3 {
			t.Fatal("processor blocked downloading")
		}
		if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL"); n != 1 {
			t.Fatalf("processed %d captures while processor was blocked", n)
		}
		close(handler.release)
		synctest.Wait()
		if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL"); n != 3 {
			t.Fatalf("queued captures were lost: processed %d", n)
		}
	})
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDownloadRetainsMalformedXMLButRejectsIncompleteTransfer(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	malformed := []byte("<broken XML from upstream>")
	s := &Service{
		store: newStore(db), config: Config{URL: testFeedURL}, trigger: make(chan struct{}, 1),
		logger: slog.New(slog.DiscardHandler),
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return feedResponse(malformed), nil
		})},
	}
	if err := s.download(t.Context()); err != nil {
		t.Fatal(err)
	}
	s.processPending(t.Context())
	if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE processed_at IS NULL"); n != 1 {
		t.Fatal("malformed XML was not retained for retry")
	}
	s.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(io.MultiReader(bytes.NewReader(atomFeed("1/a")), brokenReader{}))}, nil
	})
	if err := s.download(t.Context()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("incomplete transfer: %v", err)
	}
	if n := count(t, db, "SELECT count(*) FROM raw_feeds"); n != 1 {
		t.Fatal("incomplete transfer was stored as a complete capture")
	}
}

func TestCloseCancelsInFlightDownload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestDB(t, t.TempDir())
		started := false
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			started = true
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}
		s := New(t.Context(), db, Config{URL: testFeedURL, Interval: time.Hour, RetryDelay: time.Minute}, client, slog.New(slog.DiscardHandler))
		synctest.Wait()
		if !started {
			t.Fatal("download never started")
		}
		s.Close()
		if n := count(t, db, "SELECT count(*) FROM raw_feeds"); n != 0 {
			t.Fatal("canceled download persisted a capture")
		}
	})
}
