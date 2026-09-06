package metadata

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// Drive individual batches directly when testing durable transitions, without
// coordinating timers or relying on the worker winning a scheduling race.
func batchService(t *testing.T, db *sql.DB, transport http.RoundTripper) *Service {
	t.Helper()
	return &Service{store: &store{db: db, q: dbgen.New(db)}, client: apiClient(t, transport), logger: slog.New(slog.DiscardHandler)}
}

func fetchJob(t *testing.T, s *Service, refs ...panda.GalleryRef) FetchJob {
	t.Helper()
	job, err := s.RequestFetch(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func readFetchJob(t *testing.T, s *Service, id string) FetchJob {
	t.Helper()
	job, err := s.GetFetchJob(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func collectBatch(t *testing.T, s *Service) {
	t.Helper()
	if delay, err := s.collect(t.Context()); err != nil || delay != 0 {
		t.Fatalf("collect: delay=%s error=%v", delay, err)
	}
}

func TestFetchJobsValidateBeforeAdmissionShareWorkAndPreserveMetadata(t *testing.T) {
	db := openDB(t, t.TempDir())
	seedRefs(t, db, 4, 90)
	var s *Service
	var overlap FetchJob
	var requested [][]int64
	s = batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ids := requestedIDs(t, req)
		requested = append(requested, ids)
		if len(requested) == 1 {
			// This arrives while the original batch is in flight.
			overlap = fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
			return metadataResponse(t,
				panda.Metadata{ID: 3, Token: "wrong-token"},
				panda.Metadata{ID: 2, Error: "Key missing, or incorrect key provided."},
				panda.Metadata{ID: 1, Token: "token1", Title: "Retained", ParentID: 91, ParentToken: "token91"}), nil
		}
		return metadataResponse(t, panda.Metadata{ID: 1, Error: "Gallery not found"}), nil
	}))
	job := fetchJob(t, s,
		panda.GalleryRef{ID: 1, Token: "token1"}, panda.GalleryRef{ID: 2, Token: "invalid"},
		panda.GalleryRef{ID: 3, Token: "token3"}, panda.GalleryRef{ID: 4, Token: "conflicting"})
	if job.Status != "pending" || job.Entries[3].Error != "token_conflict" {
		t.Fatalf("submission = %+v", job)
	}
	var admitted int
	if err := db.QueryRow(`SELECT count(*) FROM gallery_refs WHERE gallery_id IN (1,2,3)`).Scan(&admitted); err != nil || admitted != 0 {
		t.Fatalf("unvalidated references admitted: %d, %v", admitted, err)
	}
	collectBatch(t, s)
	job = readFetchJob(t, s, job.ID)
	shared := readFetchJob(t, s, overlap.ID)
	if !reflect.DeepEqual(requested, [][]int64{{1, 2, 3}}) || job.Status != "completed" || shared.Status != "completed" ||
		job.Entries[0].Status != "successful" || job.Entries[1].Status != "failed" || job.Entries[2].Error != "token_mismatch" ||
		!job.Entries[0].RefreshedAt.Equal(*shared.Entries[0].RefreshedAt) {
		t.Fatalf("outcomes: job=%+v overlap=%+v requests=%v", job, shared, requested)
	}
	if err := db.QueryRow(`SELECT count(*) FROM gallery_refs WHERE gallery_id IN (2,3)`).Scan(&admitted); err != nil || admitted != 0 {
		t.Fatalf("invalid references admitted: %d, %v", admitted, err)
	}
	if token, err := s.store.q.GetGalleryToken(t.Context(), 91); err != nil || token != "token91" {
		t.Fatalf("related discovery missing: %q, %v", token, err)
	}
	result, err := s.Lookup(t.Context(), []int64{2, 1, 3})
	if err != nil || len(result.Galleries) != 1 || result.Galleries[0].Metadata.Title != "Retained" || !reflect.DeepEqual(result.UnknownIDs, []int64{2, 3}) {
		t.Fatalf("stored lookup = %+v, %v", result, err)
	}
	refresh := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
	collectBatch(t, s)
	if readFetchJob(t, s, refresh.ID).Entries[0].Status != "failed" || !reflect.DeepEqual(requested, [][]int64{{1, 2, 3}, {1}}) {
		t.Fatalf("explicit refresh did not fetch again: %v", requested)
	}
	retained, err := s.Get(t.Context(), 1)
	if err != nil || !reflect.DeepEqual(retained, result.Galleries[0]) {
		t.Fatalf("failed refresh changed retained metadata: %+v, %v", retained, err)
	}
	if readFetchJob(t, s, job.ID).Entries[0].Status != "successful" {
		t.Fatal("later refresh changed a completed job")
	}
}

func TestThousandReferenceJobResumesInBatchesBeforeFeedWork(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir)
	seedRefs(t, db, 1)
	var sizes []int
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ids := requestedIDs(t, req)
		sizes = append(sizes, len(ids))
		entries := make([]panda.Metadata, len(ids))
		for i, id := range ids {
			if id == 1 {
				t.Fatal("feed work selected before explicit job completed")
			}
			entries[i] = panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)}
		}
		return metadataResponse(t, entries...), nil
	})
	s := batchService(t, db, transport)
	refs := make([]panda.GalleryRef, MaxFetchSize)
	for i := range refs {
		refs[i] = panda.GalleryRef{ID: int64(i + 100), Token: fmt.Sprintf("token%d", i+100)}
	}
	job := fetchJob(t, s, refs...)
	shared := fetchJob(t, s, refs[:2]...)
	collectBatch(t, s)
	if readFetchJob(t, s, shared.ID).Status != "completed" || readFetchJob(t, s, job.ID).Status != "pending" {
		t.Fatal("overlapping jobs did not complete independently")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, dir)
	s = batchService(t, db, transport)
	for range 39 {
		collectBatch(t, s)
	}
	job = readFetchJob(t, s, job.ID)
	if job.Status != "completed" || len(job.Entries) != MaxFetchSize || len(sizes) != 40 {
		t.Fatalf("job=%s entries=%d batches=%v", job.Status, len(job.Entries), sizes)
	}
	for _, size := range sizes {
		if size != panda.MaxBatchSize {
			t.Fatalf("upstream batch size = %d", size)
		}
	}
	ids := make([]int64, collectorapi.MaxLookupSize)
	for i := range ids {
		ids[i] = refs[i].ID
	}
	if result, err := s.Lookup(t.Context(), ids); err != nil || len(result.Galleries) != collectorapi.MaxLookupSize {
		t.Fatalf("100-gallery lookup: %+v, %v", result, err)
	}
	if err := s.store.maintainJobs(t.Context(), job.CompletedAt.Add(JobRetention)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFetchJob(t.Context(), job.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired job still available: %v", err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT count(*) FROM metadata_fetches`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("orphan fetches remain: %d, %v", remaining, err)
	}
	if _, err := s.Get(t.Context(), refs[0].ID); err != nil {
		t.Fatalf("job cleanup removed metadata: %v", err)
	}
}

func TestInvalidBatchesLeaveNoJobs(t *testing.T) {
	db := openDB(t, t.TempDir())
	s := batchService(t, db, nil)
	for _, refs := range [][]panda.GalleryRef{
		nil, {{ID: 0, Token: "token"}}, {{ID: 1}}, {{ID: 1, Token: " "}},
		{{ID: 1, Token: "a"}, {ID: 1, Token: "b"}},
		make([]panda.GalleryRef, MaxFetchSize+1),
	} {
		if _, err := s.RequestFetch(t.Context(), refs); !errors.Is(err, ErrInvalidBatch) {
			t.Fatalf("invalid batch accepted: %v", err)
		}
	}
	for _, ids := range [][]int64{nil, {0}, {1, 1}, make([]int64, collectorapi.MaxLookupSize+1)} {
		if _, err := s.Lookup(t.Context(), ids); !errors.Is(err, ErrInvalidBatch) {
			t.Fatalf("invalid lookup accepted: %v", err)
		}
	}
	var jobs int
	if err := db.QueryRow(`SELECT count(*) FROM metadata_fetch_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("invalid submissions persisted: %d, %v", jobs, err)
	}
}

func TestQueuedFetchObeysCooldownAcrossRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		db := openDB(t, dir)
		var requests atomic.Int64
		client := apiClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if requests.Add(1) == 1 {
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader("busy"))}, nil
			}
			return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
		}))
		s := New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		job := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
		synctest.Wait()
		if readFetchJob(t, s, job.ID).Status != "pending" {
			t.Fatal("transient failure finished job")
		}
		s.Close()
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDB(t, dir)
		s = New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		shared := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
		time.Sleep(119 * time.Second)
		synctest.Wait()
		if requests.Load() != 1 {
			t.Fatalf("submission/restart bypassed cooldown: %d requests", requests.Load())
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if requests.Load() != 2 || readFetchJob(t, s, job.ID).Status != "completed" || readFetchJob(t, s, shared.ID).Status != "completed" {
			t.Fatalf("jobs did not recover: %d requests", requests.Load())
		}
	})
}

func TestFailedSuppliedTokenDoesNotParkDifferentDiscoveredToken(t *testing.T) {
	db := openDB(t, t.TempDir())
	var requested [][]int64
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requested = append(requested, requestedIDs(t, req))
		if len(requested) == 1 {
			// Related discovery happens before processing the failed entry.
			return metadataResponse(t,
				panda.Metadata{ID: 2, Token: "token2", ParentID: 1, ParentToken: "token1"},
				panda.Metadata{ID: 1, Error: "Incorrect key"}), nil
		}
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
	}))
	job := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "wrong"}, panda.GalleryRef{ID: 2, Token: "token2"})
	collectBatch(t, s)
	collectBatch(t, s)
	if !reflect.DeepEqual(requested, [][]int64{{1, 2}, {1}}) || readFetchJob(t, s, job.ID).Entries[0].Status != "failed" {
		t.Fatalf("failed supplied token interfered with discovery: %v", requested)
	}
	if got, err := s.Get(t.Context(), 1); err != nil || got.Metadata.Token != "token1" {
		t.Fatalf("discovered reference was not collected: %+v, %v", got, err)
	}
}

func TestTokenConflictDiscoveredWhileBatchIsInFlight(t *testing.T) {
	db := openDB(t, t.TempDir())
	var calls int
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		// A feed establishes the immutable inventory token during the request.
		seedRefs(t, db, 1)
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "supplied"}), nil
	}))
	job := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "supplied"})
	collectBatch(t, s)
	job = readFetchJob(t, s, job.ID)
	if calls != 1 || job.Status != "completed" || job.Entries[0].Error != "token_conflict" {
		t.Fatalf("late token conflict: %+v, calls=%d", job, calls)
	}
	if _, err := s.Get(t.Context(), 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("conflicting token stored metadata: %v", err)
	}
	rows, err := s.store.q.PendingRefs(t.Context(), 25)
	if err != nil || len(rows) != 1 || rows[0].Token != "token1" {
		t.Fatalf("feed token altered or parked: %+v, %v", rows, err)
	}
}

func TestJobCompletionAndValidatedMetadataCommitTogether(t *testing.T) {
	db := openDB(t, t.TempDir())
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
	}))
	job := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
	if _, err := db.Exec(`CREATE TRIGGER reject_completion BEFORE UPDATE ON metadata_fetch_jobs
		WHEN NEW.completed_at IS NOT NULL BEGIN SELECT RAISE(FAIL, 'storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.collect(t.Context()); err == nil {
		t.Fatal("completion unexpectedly succeeded")
	}
	if _, err := s.store.q.GetGalleryToken(t.Context(), 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("reference admission escaped rollback: %v", err)
	}
	if _, err := s.Get(t.Context(), 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("metadata escaped rollback: %v", err)
	}
	if readFetchJob(t, s, job.ID).Entries[0].Status != "pending" {
		t.Fatal("failed transaction completed job entry")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_completion`); err != nil {
		t.Fatal(err)
	}
	collectBatch(t, s)
	if readFetchJob(t, s, job.ID).Status != "completed" {
		t.Fatal("job did not recover")
	}
}
