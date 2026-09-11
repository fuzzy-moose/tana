package sitemap

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/sitemap/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("../../panda/testdata/sitemap/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func gzipped(t *testing.T, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := f(r)
	if resp != nil {
		resp.Request = r
	}
	return resp, err
}

func response(status int, body []byte, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: headers}
}

func indexBody(paths ...string) []byte {
	var b strings.Builder
	b.WriteString("<sitemapindex>")
	for _, path := range paths {
		fmt.Fprintf(&b, "<sitemap><loc>https://example.invalid%s</loc></sitemap>", path)
	}
	b.WriteString("</sitemapindex>")
	return []byte(b.String())
}

func manualService(t *testing.T, transport http.RoundTripper) *Service {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Service{db: db, q: dbgen.New(db), cfg: Config{URL: "https://example.invalid/custom.xml?revision=4"},
		client: &http.Client{Transport: transport}, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		ctx: t.Context(), cancel: func() {}, wake: make(chan struct{}, 1), retryDelay: func(int64, error, time.Time) time.Duration { return 0 }}
}

func step(t *testing.T, s *Service) {
	t.Helper()
	worked, err := s.step()
	if err != nil || !worked {
		t.Fatalf("step: worked=%v err=%v", worked, err)
	}
}

func finish(t *testing.T, s *Service) {
	t.Helper()
	for range 12 {
		state, err := s.Status(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if state.State != "running" {
			return
		}
		step(t, s)
	}
	t.Fatal("run did not finish")
}

func TestImportConditionalSkipAndForceRefresh(t *testing.T) {
	var indexRequests, childRequests int
	var conditional []bool
	child := gzipped(t, fixture(t, "child.xml"))
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/custom.xml" {
			indexRequests++
			if r.URL.RawQuery != "revision=4" {
				t.Errorf("index URL was changed: %s", r.URL)
			}
			return response(200, indexBody("/child.xml.gz"), nil), nil
		}
		childRequests++
		conditional = append(conditional, r.Header.Get("If-None-Match") == `"v1"` && r.Header.Get("If-Modified-Since") == "Thu, 10 Sep 2026 20:44:28 GMT")
		if conditional[len(conditional)-1] {
			return response(304, nil, nil), nil
		}
		return response(200, child, http.Header{"Etag": {`"v1"`}, "Last-Modified": {"Thu, 10 Sep 2026 20:44:28 GMT"}}), nil
	}))
	for run, force := range []bool{false, false, true} {
		started, err := s.Start(t.Context(), force)
		if err != nil {
			t.Fatal(err)
		}
		coalesced, err := s.Start(t.Context(), !force)
		if err != nil || coalesced.Force != force || !coalesced.StartedAt.Equal(*started.StartedAt) {
			t.Fatalf("coalesce = %+v, %v", coalesced, err)
		}
		finish(t, s)
		status, _ := s.Status(t.Context())
		if status.State != "completed" || status.ChildrenTotal != 1 {
			t.Fatalf("status = %+v", status)
		}
		if run == 0 && (status.ReferencesFound != 2 || status.ReferencesImported != 2) {
			t.Fatalf("first import = %+v", status)
		}
		if run == 1 && (status.ChildrenSkipped != 1 || status.ChildrenCompleted != 0 || status.ReferencesImported != 0) {
			t.Fatalf("conditional import = %+v", status)
		}
		if run == 2 && (status.ChildrenCompleted != 1 || status.ReferencesImported != 0) {
			t.Fatalf("force import = %+v", status)
		}
	}
	if indexRequests != 3 || childRequests != 3 || conditional[0] || !conditional[1] || conditional[2] {
		t.Fatalf("requests: index=%d child=%d conditional=%v", indexRequests, childRequests, conditional)
	}
	var refs, backfill int
	if err := s.db.QueryRow("SELECT COUNT(*), SUM(metadata_priority = 1) FROM gallery_refs").Scan(&refs, &backfill); err != nil || refs != 2 || backfill != 2 {
		t.Fatalf("refs=%d backfill=%d err=%v", refs, backfill, err)
	}
}

func TestConditionalValidatorsFollowResponseURL(t *testing.T) {
	for _, tt := range []struct {
		name        string
		firstTarget string
	}{
		{name: "redirect_target_changes", firstTarget: "/first"},
		{name: "resource_starts_redirecting", firstTarget: "/child"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.firstTarget
			var conditional []bool
			s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/custom.xml" {
					return response(200, indexBody("/child"), nil), nil
				}
				if r.URL.Path != target {
					return response(302, nil, http.Header{"Location": {target}}), nil
				}
				etag, modified := r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
				conditional = append(conditional, etag != "" || modified != "")
				if conditional[len(conditional)-1] {
					return response(304, nil, nil), nil
				}
				body := fixture(t, "child.xml")
				if target == "/second" {
					body = bytes.ReplaceAll(body, []byte("/100/"), []byte("/102/"))
				}
				return response(200, body, http.Header{"Etag": {`"revision-1"`}, "Last-Modified": {"Thu, 10 Sep 2026 20:44:28 GMT"}}), nil
			}))
			for run := range 5 {
				if run == 2 {
					target = "/second"
				}
				if _, err := s.Start(t.Context(), run == 4); err != nil {
					t.Fatal(err)
				}
				finish(t, s)
				status, err := s.Status(t.Context())
				if err != nil || status.State != "completed" {
					t.Fatalf("run %d: status=%+v err=%v", run, status, err)
				}
				if run == 2 && (status.ChildrenCompleted != 1 || status.ReferencesImported != 1) {
					t.Fatalf("changed redirect target was not imported: %+v", status)
				}
			}
			if len(conditional) != 5 || conditional[0] || !conditional[1] || conditional[2] || !conditional[3] || conditional[4] {
				t.Fatalf("conditional target requests = %v", conditional)
			}
		})
	}
}

func TestBrokenChildPreservesPartialImportsAndValidators(t *testing.T) {
	body := fixture(t, "child.xml")
	version := `"v1"`
	var requestValidator string
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/custom.xml" {
			return response(200, indexBody("/child"), nil), nil
		}
		requestValidator = r.Header.Get("If-None-Match")
		return response(200, body, http.Header{"Etag": {version}}), nil
	}))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	body, version = bytes.ReplaceAll(fixture(t, "broken.xml"), []byte("/100/"), []byte("/102/")), `"v2"`
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	status, _ := s.Status(t.Context())
	if status.State != "incomplete" || status.ChildrenFailed != 1 || status.ReferencesImported != 1 || status.ReferencesFound != 1 {
		t.Fatalf("partial status = %+v", status)
	}
	validator, err := s.q.GetValidator(t.Context(), "https://example.invalid/child")
	if err != nil || validator.Etag != `"v1"` || requestValidator != `"v1"` {
		t.Fatalf("validator = %+v sent=%s err=%v", validator, requestValidator, err)
	}
	var token string
	if err := s.db.QueryRow("SELECT token FROM gallery_refs WHERE gallery_id = 102").Scan(&token); err != nil {
		t.Fatalf("partial reference missing: %v", err)
	}
	body = fixture(t, "child.xml")
	if _, err := s.Retry(t.Context()); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	status, _ = s.Status(t.Context())
	validator, _ = s.q.GetValidator(t.Context(), "https://example.invalid/child")
	if status.State != "completed" || status.ReferencesImported != 1 || validator.Etag != `"v2"` {
		t.Fatalf("retry status=%+v validator=%+v", status, validator)
	}
}

func TestBoundedFailuresContinueOtherChildrenAndRetryOnlyFailures(t *testing.T) {
	requests := map[string]int{}
	failing := true
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests[r.URL.Path]++
		if r.URL.Path == "/custom.xml" {
			return response(200, indexBody("/bad", "/good"), nil), nil
		}
		if r.URL.Path == "/bad" && failing {
			return response(503, nil, nil), nil
		}
		return response(200, fixture(t, "child.xml"), nil), nil
	}))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	step(t, s) // index
	step(t, s) // first bad attempt
	step(t, s) // untouched good child precedes retry
	if requests["/good"] != 1 || requests["/bad"] != 1 {
		t.Fatalf("did not continue: %v", requests)
	}
	finish(t, s)
	status, _ := s.Status(t.Context())
	if status.State != "incomplete" || status.ChildrenCompleted != 1 || status.ChildrenFailed != 1 || requests["/bad"] != 3 {
		t.Fatalf("status=%+v requests=%v", status, requests)
	}
	failing = false
	if _, err := s.Retry(t.Context()); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	status, _ = s.Status(t.Context())
	if status.State != "completed" || requests["/custom.xml"] != 1 || requests["/good"] != 1 || requests["/bad"] != 4 {
		t.Fatalf("retry status=%+v requests=%v", status, requests)
	}
}

func TestIndexFailuresAreBounded(t *testing.T) {
	var requests int
	s := manualService(t, roundTripFunc(func(*http.Request) (*http.Response, error) { requests++; return response(503, nil, nil), nil }))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	status, _ := s.Status(t.Context())
	if status.State != "incomplete" || requests != 3 || status.LastError != "upstream_http_503" {
		t.Fatalf("status=%+v requests=%d", status, requests)
	}
}

func TestCancellationStopsRequestAndRetryKeepsCompletedChildren(t *testing.T) {
	started := make(chan struct{})
	block := true
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/custom.xml" {
			return response(200, indexBody("/done", "/blocked"), nil), nil
		}
		if r.URL.Path == "/blocked" && block {
			close(started)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return response(200, fixture(t, "child.xml"), nil), nil
	}))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	step(t, s)
	step(t, s)
	done := make(chan error, 1)
	go func() { _, err := s.step(); done <- err }()
	<-started
	status, err := s.Cancel(t.Context())
	if err != nil || status.State != "cancelled" || status.ChildrenCompleted != 1 || status.ReferencesImported != 2 {
		t.Fatalf("cancel=%+v err=%v", status, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	block = false
	if _, err := s.Retry(t.Context()); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	status, _ = s.Status(t.Context())
	if status.State != "completed" || status.ChildrenCompleted != 2 || status.ReferencesImported != 2 {
		t.Fatalf("retry=%+v", status)
	}
}

func TestRestartRecoversInterruptedChildWithoutRefetchingIndex(t *testing.T) {
	var requests []string
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path == "/custom.xml" {
			return response(200, indexBody("/done", "/interrupted"), nil), nil
		}
		return response(200, fixture(t, "child.xml"), nil), nil
	}))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	step(t, s)
	step(t, s)
	child, err := s.q.NextChild(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	child.State, child.Failures = "running", 1
	if err := updateChild(t.Context(), s.q, child); err != nil {
		t.Fatal(err)
	}
	// The previous deadline can expire while a redirected request is still in
	// flight. Restart must allow a full interval before the next request.
	run, _ := s.q.GetRun(t.Context())
	run.NextRequestAt = time.Now().Add(-time.Second).UnixMilli()
	if err := updateRun(t.Context(), s.q, run); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(t.Context(), s.db, s.cfg, s.client, s.logger)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Close()
	recoveredRun, err := s.q.GetRun(t.Context())
	if err != nil || recoveredRun.NextRequestAt <= time.Now().UnixMilli() {
		t.Fatalf("restart did not restore request spacing: %+v, %v", recoveredRun, err)
	}
	child, err = s.q.NextChild(t.Context())
	if err != nil || child.State != "pending" || child.Failures != 1 {
		t.Fatalf("recovered=%+v err=%v", child, err)
	}
	run.NextRequestAt = 0
	if err := updateRun(t.Context(), s.q, run); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	if strings.Join(requests, ",") != "/custom.xml,/done,/interrupted" {
		t.Fatalf("requests=%v", requests)
	}
}

func TestRetryAfterAndBanPersistGlobalPauseWithoutConsumingBanAttempts(t *testing.T) {
	for _, ban := range []bool{false, true} {
		t.Run(fmt.Sprintf("ban=%v", ban), func(t *testing.T) {
			var requestedChild bool
			until := time.Now().Add(3 * time.Minute)
			s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/custom.xml" {
					return response(200, indexBody("/child", "/next"), nil), nil
				}
				requestedChild = true
				if ban {
					return nil, &panda.BanError{Until: until}
				}
				return response(429, nil, http.Header{"Retry-After": {"180"}}), nil
			}))
			s.retryDelay = panda.RetryDelay
			if _, err := s.Start(t.Context(), false); err != nil {
				t.Fatal(err)
			}
			step(t, s)
			step(t, s)
			run, _ := s.q.GetRun(t.Context())
			if !requestedChild || run.NextRequestAt < until.Add(-time.Second).UnixMilli() {
				t.Fatalf("pause=%+v", run)
			}
			if worked, err := s.step(); err != nil || worked {
				t.Fatalf("made request during cooldown: %v, %v", worked, err)
			}
			var failures int64
			if err := s.db.QueryRow("SELECT failures FROM sitemap_children WHERE url = 'https://example.invalid/child'").Scan(&failures); err != nil {
				t.Fatal(err)
			}
			if ban && failures != 0 || !ban && failures != 1 {
				t.Fatalf("failures=%d", failures)
			}
		})
	}
}

func TestInvalidEntriesAreCountedWithoutFailingChild(t *testing.T) {
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/custom.xml" {
			return response(200, indexBody("/child"), nil), nil
		}
		return response(200, []byte(`<urlset><url><loc>invalid</loc></url><url/><url><loc>https://e-hentai.org/g/100/012345678a/</loc></url></urlset>`), nil), nil
	}))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	status, _ := s.Status(t.Context())
	if status.State != "completed" || status.InvalidLocations != 2 || status.ReferencesFound != 1 || status.ReferencesImported != 1 {
		t.Fatalf("status=%+v", status)
	}
}

func TestUnexpected304DoesNotCreateValidator(t *testing.T) {
	s := manualService(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/custom.xml" {
			return response(200, indexBody("/child"), nil), nil
		}
		return response(304, nil, nil), nil
	}))
	if _, err := s.Start(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	finish(t, s)
	_, err := s.q.GetValidator(t.Context(), "https://example.invalid/child")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("validator error=%v", err)
	}
	status, _ := s.Status(t.Context())
	if status.State != "incomplete" {
		t.Fatalf("status=%+v", status)
	}
}
