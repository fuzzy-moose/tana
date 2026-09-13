package metadata

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestBackgroundReservationsShareWorkWithoutChangingMainCooldown(t *testing.T) {
	db := openDB(t, t.TempDir())
	for id := int64(1); id <= 76; id++ {
		seedRefs(t, db, id)
	}
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var entries []panda.Metadata
		for _, id := range requestedIDs(t, req) {
			entries = append(entries, panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)})
		}
		return metadataResponse(t, entries...), nil
	}))
	job := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
	main, err := s.claim(t.Context(), false)
	if err != nil || main.Size() != 1 || main.refs[0].ID != 1 {
		t.Fatalf("main reservation: %+v, %v", main, err)
	}
	defer main.Close()
	seen := map[int64]bool{1: true}
	var batches []*Batch
	for range 3 {
		batch, err := s.ClaimBackground(t.Context())
		if err != nil || batch == nil || batch.Size() != 25 {
			t.Fatalf("background reservation: %+v, %v", batch, err)
		}
		defer batch.Close()
		batches = append(batches, batch)
		for _, ref := range batch.refs {
			if seen[ref.ID] {
				t.Fatalf("gallery %d reserved twice", ref.ID)
			}
			seen[ref.ID] = true
		}
	}
	if extra, err := s.ClaimBackground(t.Context()); err != nil || extra != nil {
		t.Fatalf("reserved work selected again: %+v, %v", extra, err)
	}
	until := time.Now().Add(time.Hour).UnixMilli()
	if _, err := db.Exec(`UPDATE metadata_retry SET failures = 3, next_attempt_at = ?`, until); err != nil {
		t.Fatal(err)
	}
	// A new explicit owner can share a proxy batch already in flight.
	shared := fetchJob(t, s, batches[0].refs[0])
	for _, batch := range batches {
		if err := batch.Fetch(t.Context(), s.client); err != nil {
			t.Fatal(err)
		}
	}
	var failures, deadline int64
	if err := db.QueryRow(`SELECT failures, next_attempt_at FROM metadata_retry`).Scan(&failures, &deadline); err != nil || failures != 3 || deadline != until {
		t.Fatalf("proxy completion changed main cooldown: %d/%d, %v", failures, deadline, err)
	}
	if got := readFetchJob(t, s, shared.ID); got.Status != "completed" || got.Entries[0].Status != "successful" {
		t.Fatalf("shared job: %+v", got)
	}
	if got := readFetchJob(t, s, job.ID); got.Status != "pending" {
		t.Fatalf("reserved main job: %+v", got)
	}
}

func TestBackgroundImportsWaitForInventoryButNotExplicitJobs(t *testing.T) {
	db := openDB(t, t.TempDir())
	s := batchService(t, db, nil)
	imports := batchImports(t, db, t.TempDir())
	acceptImport(t, imports, "99,token99\n100,token100")
	parseImports(t, imports)
	seedRefs(t, db, 1)
	fetchJob(t, s, panda.GalleryRef{ID: 88, Token: "explicit88"})
	inventory, err := s.ClaimBackground(t.Context())
	if err != nil || inventory == nil || inventory.refs[0].ID != 1 {
		t.Fatalf("inventory: %+v, %v", inventory, err)
	}
	defer inventory.Close()
	if batch, err := s.ClaimBackground(t.Context()); err != nil || batch != nil {
		t.Fatalf("imports bypassed in-flight inventory: %+v, %v", batch, err)
	}
	if _, err := db.Exec(`UPDATE gallery_refs SET metadata_attempted_at = 1`); err != nil {
		t.Fatal(err)
	}
	// Reserve one imported gallery through the main explicit queue as well.
	fetchJob(t, s, panda.GalleryRef{ID: 99, Token: "other99"})
	main, err := s.claim(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer main.Close()
	batch, err := s.ClaimBackground(t.Context())
	if err != nil || batch == nil || batch.Size() != 1 || batch.refs[0].ID != 100 {
		t.Fatalf("independent import reservation: %+v, %v", batch, err)
	}
	batch.Close()
}
