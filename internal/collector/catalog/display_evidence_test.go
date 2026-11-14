package catalog

import (
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func displayEvidence(t *testing.T, db *pgxpool.Pool) map[int64]bool {
	t.Helper()
	rows, err := db.Query(t.Context(), "SELECT gallery_id, evidence FROM panda_gallery_displays")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	evidence := map[int64]bool{}
	for rows.Next() {
		var id int64
		var e bool
		if err := rows.Scan(&id, &e); err != nil {
			t.Fatal(err)
		}
		evidence[id] = e
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return evidence
}

func TestRecordDisplaysStoresEvidenceByScoreFilter(t *testing.T) {
	db, s := openCatalog(t)
	for _, record := range []struct {
		id     int64
		filter string
	}{{1, ""}, {2, collectorapi.ScoreFilterAll}, {3, collectorapi.ScoreFilterHidden}} {
		if err := s.RecordDisplays(t.Context(), []int64{record.id}, record.filter); err != nil {
			t.Fatalf("%q: %v", record.filter, err)
		}
	}
	if err := s.RecordDisplays(t.Context(), []int64{4}, "bogus"); !errors.Is(err, ErrInvalidDisplays) {
		t.Fatalf("bogus filter: %v", err)
	}
	setVerdict(t, s, 5, collectorapi.VerdictNotForMe)
	if got, want := displayEvidence(t, db), map[int64]bool{1: true, 2: true, 3: false, 5: true}; !maps.Equal(got, want) {
		t.Fatalf("evidence = %v, want %v", got, want)
	}
}

func TestHiddenOnlyDisplayStillSeesTheGallery(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "One", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "Two", Posted: 20}, true)
	if err := s.RecordDisplays(t.Context(), []int64{2}, collectorapi.ScoreFilterHidden); err != nil {
		t.Fatal(err)
	}
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{}); !maps.Equal(got, map[int64]bool{1: false, 2: true}) {
		t.Fatalf("displayed: %v", got)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{1})
}
