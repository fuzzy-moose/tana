package metadata

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestFeedDelayAppliesBeforeLimitAcrossChannels(t *testing.T) {
	db := openDB(t)
	s := batchService(t, db, nil)
	now := time.Now()
	for id := int64(1); id <= 32; id++ {
		seedRefs(t, db, id)
	}
	if _, err := db.Exec(t.Context(), `UPDATE gallery_refs SET feed_metadata_base_at = $1 WHERE gallery_id <= 30`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE gallery_refs SET feed_metadata_base_at = $1 WHERE gallery_id = 32`, now.Add(-13*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, proxy := range []bool{false, true} {
		batch, err := s.claim(t.Context(), proxy)
		if err != nil || batch == nil {
			t.Fatalf("proxy=%v: %v, %v", proxy, batch, err)
		}
		var ids []int64
		for _, ref := range batch.refs {
			ids = append(ids, ref.ID)
		}
		batch.Close()
		if !reflect.DeepEqual(ids, []int64{31, 32}) {
			t.Fatalf("proxy=%v eligible refs: %v", proxy, ids)
		}
	}
	// Automatic rediscovery raises priority but must preserve the established delay.
	if err := s.store.q.SaveGalleryRef(t.Context(), dbgen.SaveGalleryRefParams{GalleryID: 1, Token: "token1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(t.Context(), collectorapi.MetadataSettings{FeedMetadataDelayHours: 0}); err != nil {
		t.Fatal(err)
	}
	batch, err := s.ClaimBackground(t.Context())
	if err != nil || batch == nil {
		t.Fatalf("zero delay claim: %v, %v", batch, err)
	}
	defer batch.Close()
	if batch.Size() != panda.MaxBatchSize || batch.refs[0].ID != 1 {
		t.Fatalf("setting did not apply to waiting work: %+v", batch.refs)
	}
}

func TestExplicitFetchBypassesFeedDelayWithoutLaterRefetch(t *testing.T) {
	db := openDB(t)
	seedRefs(t, db, 1)
	if _, err := db.Exec(t.Context(), `UPDATE gallery_refs SET feed_metadata_base_at = $1`, time.Now()); err != nil {
		t.Fatal(err)
	}
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1", Title: "Settled"}), nil
	}))
	job := fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"})
	if batch, err := s.ClaimBackground(t.Context()); err != nil || batch != nil {
		t.Fatalf("proxy bypassed delay: %v, %v", batch, err)
	}
	batch, err := s.claim(t.Context(), false)
	if err != nil || batch == nil || batch.Size() != 1 {
		t.Fatalf("explicit job blocked: %v, %v", batch, err)
	}
	defer batch.Close()
	if err := batch.Fetch(t.Context(), s.client); err != nil {
		t.Fatal(err)
	}
	batch.Close()
	if got := readFetchJob(t, s, job.ID); got.Status != "completed" {
		t.Fatalf("job: %+v", got)
	}
	if _, err := db.Exec(t.Context(), `UPDATE gallery_refs SET feed_metadata_base_at = $1`, time.Now().Add(-13*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, proxy := range []bool{false, true} {
		if batch, err := s.claim(t.Context(), proxy); err != nil || batch != nil {
			t.Fatalf("proxy=%v fetched again after deadline: %v, %v", proxy, batch, err)
		}
	}
}

func TestFeedDelaySettingsPersistAndValidate(t *testing.T) {
	db := openDB(t)
	s := batchService(t, db, nil)
	var hours int64
	if err := db.QueryRow(t.Context(), `SELECT feed_metadata_delay_hours FROM metadata_collection_settings`).Scan(&hours); err != nil || hours != 12 {
		t.Fatalf("default=%d, %v", hours, err)
	}
	for _, hours := range []int64{-1, collectorapi.MaxFeedMetadataDelayHours + 1} {
		if err := s.SetSettings(t.Context(), collectorapi.MetadataSettings{FeedMetadataDelayHours: hours}); !errors.Is(err, collectorapi.ErrInvalidMetadataSettings) {
			t.Fatalf("accepted %d: %v", hours, err)
		}
	}
	if err := s.SetMainBackgroundPaused(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(t.Context(), collectorapi.MetadataSettings{FeedMetadataDelayHours: 24}); err != nil {
		t.Fatal(err)
	}
	db = pgtest.Reopen(t, db, storage.Migrations())
	var paused bool
	if err := db.QueryRow(t.Context(), `SELECT feed_metadata_delay_hours, main_background_paused FROM metadata_collection_settings`).Scan(&hours, &paused); err != nil || hours != 24 || !paused {
		t.Fatalf("reopened settings: %d, %v, %v", hours, paused, err)
	}
}
