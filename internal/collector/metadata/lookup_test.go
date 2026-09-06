package metadata

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestLookupDistinguishesCollectionStatesWithoutScheduling(t *testing.T) {
	db := openDB(t, t.TempDir())
	seedRefs(t, db, 1, 2, 3, 4, 6)
	s := batchService(t, db, nil)
	for _, id := range []int64{1, 3, 4, 6} {
		if err := s.store.q.RecordAttempt(t.Context(), dbgen.RecordAttemptParams{
			GalleryID: id, Token: "token" + strconv.FormatInt(id, 10),
			MetadataAttemptedAt: sql.NullInt64{Int64: 1000, Valid: true}, MetadataError: sql.NullString{String: "unavailable", Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(panda.Metadata{ID: 1, Token: "token1", Title: "Retained"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.q.SaveMetadata(t.Context(), dbgen.SaveMetadataParams{GalleryID: 1, Body: body, RefreshedAt: 100}); err != nil {
		t.Fatal(err)
	}
	// Retained metadata beats a pending refresh; a known failed reference with a
	// matching retry is pending. Unvalidated submissions remain unknown.
	fetchJob(t, s, panda.GalleryRef{ID: 1, Token: "token1"}, panda.GalleryRef{ID: 4, Token: "token4"}, panda.GalleryRef{ID: 5, Token: "unvalidated"})
	// Even a conflicting fetch not yet maintained cannot make inventory pending.
	if _, err := s.store.q.CreateFetch(t.Context(), dbgen.CreateFetchParams{GalleryID: 6, Token: "wrong", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Lookup(t.Context(), []int64{1, 2, 3, 4, 5, 6, 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Galleries) != 1 || result.Galleries[0].Metadata.Title != "Retained" || result.Galleries[0].RefreshedAt.UnixMilli() != 100 ||
		!reflect.DeepEqual(result.PendingIDs, []int64{2, 4}) || !reflect.DeepEqual(result.FailedIDs, []int64{3, 6}) || !reflect.DeepEqual(result.UnknownIDs, []int64{5, 7}) {
		t.Fatalf("lookup states: %+v", result)
	}
	var jobs, fetches int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM metadata_fetch_jobs), (SELECT count(*) FROM metadata_fetches)`).Scan(&jobs, &fetches); err != nil || jobs != 1 || fetches != 4 {
		t.Fatalf("lookup changed work: jobs=%d fetches=%d error=%v", jobs, fetches, err)
	}
}
