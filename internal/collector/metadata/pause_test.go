package metadata

import (
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestMainBackgroundPauseFinishesAssignedBatchAndSurvivesRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		db := openDB(t, dir)
		seedRefs(t, db, 1)
		finish := make(chan struct{})
		var requested []int64
		client := apiClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			ids := requestedIDs(t, req)
			requested = append(requested, ids...)
			if ids[0] == 1 {
				select {
				case <-finish:
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			}
			var entries []panda.Metadata
			for _, id := range ids {
				entries = append(entries, panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)})
			}
			return metadataResponse(t, entries...), nil
		}))
		s := New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		if !reflect.DeepEqual(requested, []int64{1}) {
			t.Fatalf("assigned batch: %v", requested)
		}
		if err := s.SetMainBackgroundPaused(t.Context(), true); err != nil {
			t.Fatal(err)
		}
		seedRefs(t, db, 2)
		close(finish)
		synctest.Wait()
		if _, err := s.Get(t.Context(), 1); err != nil {
			t.Fatalf("pause discarded assigned batch: %v", err)
		}
		s.Close()
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDB(t, dir)
		s = New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if !reflect.DeepEqual(requested, []int64{1}) {
			t.Fatalf("pause lost across restart: %v", requested)
		}
		job := fetchJob(t, s, panda.GalleryRef{ID: 99, Token: "token99"})
		synctest.Wait()
		if got := readFetchJob(t, s, job.ID); got.Status != "completed" || got.Entries[0].Status != "successful" {
			t.Fatalf("explicit request blocked by pause: %+v", got)
		}
		if !reflect.DeepEqual(requested, []int64{1, 99}) {
			t.Fatalf("explicit request resumed background work: %v", requested)
		}
		if err := s.SetMainBackgroundPaused(t.Context(), false); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if !reflect.DeepEqual(requested, []int64{1, 99, 2}) {
			t.Fatalf("resume did not wake pending collection: %v", requested)
		}
	})
}

func TestMainBackgroundPauseLeavesProxyCollectionAndLocalImportsAvailable(t *testing.T) {
	db := openDB(t, t.TempDir())
	seedRefs(t, db, 1)
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var entries []panda.Metadata
		for _, id := range requestedIDs(t, req) {
			entries = append(entries, panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)})
		}
		return metadataResponse(t, entries...), nil
	}))
	if err := s.SetMainBackgroundPaused(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	imports := batchImports(t, db, t.TempDir())
	item := acceptImport(t, imports, "1,token1\n2,token2")
	parseImports(t, imports)
	if got := readImport(t, imports, item.ID); got.Known != 1 || got.Pending != 1 {
		t.Fatalf("pause blocked local import processing: %+v", got)
	}
	for _, id := range []int64{1, 2} {
		if batch, err := s.claim(t.Context(), false); err != nil || batch != nil {
			t.Fatalf("main claimed background work while paused: %+v, %v", batch, err)
		}
		batch, err := s.ClaimBackground(t.Context())
		if err != nil || batch == nil || batch.Size() != 1 || batch.refs[0].ID != id {
			t.Fatalf("proxy could not claim gallery %d: %+v, %v", id, batch, err)
		}
		defer batch.Close()
		if err := batch.Fetch(t.Context(), s.client); err != nil {
			t.Fatal(err)
		}
		batch.Close()
	}
	if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Imported != 1 {
		t.Fatalf("proxy did not finish reference validation: %+v", got)
	}
}

func TestMainBackgroundResumePreservesRetryBanAndImportPause(t *testing.T) {
	db := openDB(t, t.TempDir())
	s := batchService(t, db, nil)
	imports := batchImports(t, db, t.TempDir())
	item := acceptImport(t, imports, "1,token1")
	parseImports(t, imports)
	if _, err := imports.Pause(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}
	retryAt := time.Now().Add(time.Hour).UnixMilli()
	if _, err := db.Exec(`UPDATE metadata_retry SET failures = 3, next_attempt_at = ?, last_error = 'unavailable'`, retryAt); err != nil {
		t.Fatal(err)
	}
	ban := pandaban.New(db)
	banUntil := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)
	if err := ban.Extend(t.Context(), banUntil); err != nil {
		t.Fatal(err)
	}
	for _, paused := range []bool{true, false} {
		if err := s.SetMainBackgroundPaused(t.Context(), paused); err != nil {
			t.Fatal(err)
		}
	}
	retry, err := s.store.q.RetryState(t.Context())
	if err != nil || retry.Failures != 3 || retry.NextAttemptAt != retryAt {
		t.Fatalf("resume changed retry state: %+v, %v", retry, err)
	}
	if until, err := ban.Until(t.Context()); err != nil || !until.Equal(banUntil) {
		t.Fatalf("resume changed ban: %s, %v", until, err)
	}
	if got := readImport(t, imports, item.ID); !got.Paused {
		t.Fatalf("resume cleared individual import pause: %+v", got)
	}
	if batch, err := s.claim(t.Context(), false); err != nil || batch != nil {
		t.Fatalf("resumed main claimed paused import: %+v, %v", batch, err)
	}
	seedRefs(t, db, 2)
	if delay, err := s.collect(t.Context()); err != nil || delay <= 0 {
		t.Fatalf("resume bypassed retry delay: %s, %v", delay, err)
	}
}
