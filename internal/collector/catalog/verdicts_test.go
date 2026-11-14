package catalog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func verdicts(t *testing.T, s *Service, options collectorapi.CatalogOptions) map[int64]collectorapi.CatalogItem {
	t.Helper()
	options.PageSize = 100
	result, err := s.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	items := make(map[int64]collectorapi.CatalogItem, len(result.Items))
	for _, item := range result.Items {
		items[item.GalleryID] = item
	}
	return items
}

func verdictRow(t *testing.T, db *pgxpool.Pool, id int64) (string, time.Time, bool) {
	t.Helper()
	var verdict string
	var givenAt time.Time
	err := db.QueryRow(t.Context(), "SELECT verdict, given_at FROM panda_gallery_verdicts WHERE gallery_id = $1", id).Scan(&verdict, &givenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return verdict, givenAt, true
}

func setVerdict(t *testing.T, s *Service, id int64, verdict string) {
	t.Helper()
	if err := s.SetVerdict(t.Context(), id, verdict); err != nil {
		t.Fatal(err)
	}
}

func TestVerdictIsListedAndDisplaysTheGallery(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "Red one", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "Red two", Posted: 20}, true)
	retain(t, db, panda.Metadata{ID: 3, Title: "Blue", Posted: 30}, true)
	setVerdict(t, s, 2, collectorapi.VerdictInteresting)
	for _, options := range []collectorapi.CatalogOptions{{}, {Query: "red"}} {
		items := verdicts(t, s, options)
		if items[2].Verdict != collectorapi.VerdictInteresting || !items[2].Displayed {
			t.Fatalf("%+v: gallery 2 = %+v", options, items[2])
		}
		if items[1].Verdict != "" || items[1].Displayed {
			t.Fatalf("%+v: gallery 1 = %+v", options, items[1])
		}
	}
	items := verdicts(t, s, collectorapi.CatalogOptions{})
	body, err := json.Marshal(items[3])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"verdict"`) {
		t.Fatalf("gallery without a verdict: %s", body)
	}
	if len(displayTimes(t, db, 2)) != 1 {
		t.Fatalf("displays of gallery 2: %v", displayTimes(t, db, 2))
	}
	unseen := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100})
	for _, item := range unseen.Items {
		if item.GalleryID == 2 {
			t.Fatalf("unseen view lists the gallery with a verdict: %+v", unseen.Items)
		}
	}
	if len(unseen.Items) != 2 {
		t.Fatalf("unseen view: %+v", unseen.Items)
	}
}

func TestVerdictChangeKeepsOneRow(t *testing.T) {
	db, s := openCatalog(t)
	setVerdict(t, s, 5, collectorapi.VerdictInteresting)
	_, first, _ := verdictRow(t, db, 5)
	setVerdict(t, s, 5, collectorapi.VerdictInteresting)
	if verdict, at, _ := verdictRow(t, db, 5); verdict != collectorapi.VerdictInteresting || !at.Equal(first) {
		t.Fatalf("same verdict: %s %v, first %v", verdict, at, first)
	}
	setVerdict(t, s, 5, collectorapi.VerdictNotForMe)
	if verdict, at, _ := verdictRow(t, db, 5); verdict != collectorapi.VerdictNotForMe || !at.After(first) {
		t.Fatalf("changed verdict: %s %v, first %v", verdict, at, first)
	}
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_gallery_verdicts WHERE gallery_id = 5").Scan(&count); err != nil || count != 1 {
		t.Fatalf("rows = %d, %v", count, err)
	}
}

func TestRemovedVerdictLeavesTheGalleryDisplayed(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "One", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "Two", Posted: 20}, true)
	setVerdict(t, s, 1, collectorapi.VerdictNotForMe)
	if err := s.RemoveVerdict(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if item := verdicts(t, s, collectorapi.CatalogOptions{})[1]; item.Verdict != "" || !item.Displayed {
		t.Fatalf("after removal: %+v", item)
	}
	if unseen := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}); len(unseen.Items) != 1 || unseen.Items[0].GalleryID != 2 {
		t.Fatalf("unseen after removal: %+v", unseen.Items)
	}
	if err := s.RemoveVerdict(t.Context(), 2); err != nil {
		t.Fatalf("removing a missing verdict: %v", err)
	}
}

func TestVerdictSurvivesProjectionChanges(t *testing.T) {
	db, s := openCatalog(t)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 4, Token: "token4"}, Title: "Feed"}, time.Unix(40, 0))
	setVerdict(t, s, 4, collectorapi.VerdictInteresting)
	check := func(stage string, options collectorapi.CatalogOptions) {
		t.Helper()
		if item := verdicts(t, s, options)[4]; item.Verdict != collectorapi.VerdictInteresting {
			t.Fatalf("%s: %+v", stage, item)
		}
	}
	check("provisional", collectorapi.CatalogOptions{})
	retain(t, db, panda.Metadata{ID: 4, Title: "Collected", Posted: 40}, true)
	check("collected", collectorapi.CatalogOptions{})
	if _, err := db.Exec(t.Context(), "DELETE FROM catalog_galleries WHERE gallery_id = 4"); err != nil {
		t.Fatal(err)
	}
	retain(t, db, panda.Metadata{ID: 4, Title: "Collected", Posted: 40}, true)
	check("re-projected", collectorapi.CatalogOptions{})
	retain(t, db, panda.Metadata{ID: 4, Title: "Collected", Posted: 40, Expunged: true}, true)
	check("expunged", collectorapi.CatalogOptions{IncludeExpunged: true})
}

func TestInvalidVerdictsStoreNothing(t *testing.T) {
	db, s := openCatalog(t)
	for _, id := range []int64{0, -1} {
		if err := s.SetVerdict(t.Context(), id, collectorapi.VerdictInteresting); !errors.Is(err, ErrInvalidGalleryReference) {
			t.Fatalf("set %d: %v", id, err)
		}
		if err := s.RemoveVerdict(t.Context(), id); !errors.Is(err, ErrInvalidGalleryReference) {
			t.Fatalf("remove %d: %v", id, err)
		}
	}
	for _, verdict := range []string{"", "maybe", "Interesting"} {
		if err := s.SetVerdict(t.Context(), 1, verdict); !errors.Is(err, ErrInvalidVerdict) {
			t.Fatalf("set %q: %v", verdict, err)
		}
	}
	var verdictCount, displayCount int
	if err := db.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM panda_gallery_verdicts), (SELECT count(*) FROM panda_gallery_displays)").Scan(&verdictCount, &displayCount); err != nil {
		t.Fatal(err)
	}
	if verdictCount != 0 || displayCount != 0 {
		t.Fatalf("invalid verdicts stored %d verdicts and %d displays", verdictCount, displayCount)
	}
}

func verdictPage(t *testing.T, s *Service, verdict string, options collectorapi.CatalogOptions) collectorapi.CatalogResult {
	t.Helper()
	options.Verdict = verdict
	if options.PageSize == 0 {
		options.PageSize = 100
	}
	result, err := s.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestVerdictViewListsOnlyItsVerdictInCatalogOrder(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 6; id++ {
		retain(t, db, panda.Metadata{ID: id, Title: "Gallery", Posted: id * 10}, true)
	}
	setVerdict(t, s, 2, collectorapi.VerdictInteresting)
	setVerdict(t, s, 5, collectorapi.VerdictInteresting)
	setVerdict(t, s, 3, collectorapi.VerdictNotForMe)
	setVerdict(t, s, 99, collectorapi.VerdictInteresting) // no catalog row
	interesting := verdictPage(t, s, collectorapi.VerdictInteresting, collectorapi.CatalogOptions{})
	assertCatalogIDs(t, interesting, []int64{5, 2})
	for _, item := range interesting.Items {
		if item.Verdict != collectorapi.VerdictInteresting || !item.Displayed {
			t.Fatalf("interesting item = %+v", item)
		}
	}
	if interesting.NextCursor != "" || interesting.PreviousCursor != "" {
		t.Fatalf("single page cursors = %+v", interesting)
	}
	assertCatalogIDs(t, verdictPage(t, s, collectorapi.VerdictNotForMe, collectorapi.CatalogOptions{}), []int64{3})

	setVerdict(t, s, 5, collectorapi.VerdictNotForMe)
	if err := s.RemoveVerdict(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, verdictPage(t, s, collectorapi.VerdictInteresting, collectorapi.CatalogOptions{}), []int64{})
	assertCatalogIDs(t, verdictPage(t, s, collectorapi.VerdictNotForMe, collectorapi.CatalogOptions{}), []int64{5, 3})
}

func TestVerdictViewAppliesEveryFilter(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "Red manga", Category: "Manga", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "Red doujinshi", Category: "Doujinshi", Posted: 20}, true)
	retain(t, db, panda.Metadata{ID: 3, Title: "Blue manga", Category: "Manga", Posted: 30}, true)
	retain(t, db, panda.Metadata{ID: 4, Title: "Red cosplay", Category: "Cosplay", Posted: 40}, true)
	retain(t, db, panda.Metadata{ID: 5, Title: "Red manga", Category: "Manga", Posted: 50}, true)
	retain(t, db, panda.Metadata{ID: 6, Title: "Red gone", Category: "Manga", Posted: 60, Expunged: true}, true)
	for _, id := range []int64{1, 2, 3, 4, 6} {
		setVerdict(t, s, id, collectorapi.VerdictInteresting)
	}
	for _, tc := range []struct {
		name    string
		options collectorapi.CatalogOptions
		want    []int64
	}{
		{"search", collectorapi.CatalogOptions{Query: "red"}, []int64{4, 2, 1}},
		{"one category", collectorapi.CatalogOptions{Categories: []string{"manga"}}, []int64{3, 1}},
		{"several categories", collectorapi.CatalogOptions{Categories: []string{"manga", "doujinshi"}}, []int64{3, 2, 1}},
		{"defaults", collectorapi.CatalogOptions{DefaultQuery: "red", DefaultCategories: []string{"manga", "cosplay"}}, []int64{4, 1}},
		{"expunged", collectorapi.CatalogOptions{IncludeExpunged: true, Query: "red"}, []int64{6, 4, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertCatalogIDs(t, verdictPage(t, s, collectorapi.VerdictInteresting, tc.options), tc.want)
		})
	}
}

func TestVerdictViewPagesForwardAndBack(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 12; id++ {
		retain(t, db, panda.Metadata{ID: id, Title: "Gallery", Posted: id * 10}, true)
		if id%2 == 0 {
			setVerdict(t, s, id, collectorapi.VerdictNotForMe)
		}
	}
	options := collectorapi.CatalogOptions{PageSize: 2}
	first := verdictPage(t, s, collectorapi.VerdictNotForMe, options)
	assertCatalogIDs(t, first, []int64{12, 10})
	if first.PreviousCursor != "" || first.NextCursor == "" {
		t.Fatalf("first page cursors = %+v", first)
	}
	options.Cursor = first.NextCursor
	second := verdictPage(t, s, collectorapi.VerdictNotForMe, options)
	assertCatalogIDs(t, second, []int64{8, 6})
	options.Cursor = second.NextCursor
	last := verdictPage(t, s, collectorapi.VerdictNotForMe, options)
	assertCatalogIDs(t, last, []int64{4, 2})
	if last.NextCursor != "" || last.PreviousCursor == "" {
		t.Fatalf("last page cursors = %+v", last)
	}
	options.Cursor = last.PreviousCursor
	back := verdictPage(t, s, collectorapi.VerdictNotForMe, options)
	assertCatalogIDs(t, back, []int64{8, 6})
	options.Cursor = back.PreviousCursor
	start := verdictPage(t, s, collectorapi.VerdictNotForMe, options)
	assertCatalogIDs(t, start, []int64{12, 10})
	if start.PreviousCursor != "" || start.NextCursor == "" {
		t.Fatalf("back at the start cursors = %+v", start)
	}
}

func TestVerdictViewRejectsUnknownVerdictsAndTheUnseenView(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "One", Posted: 10}, true)
	for _, options := range []collectorapi.CatalogOptions{
		{PageSize: 10, Verdict: "maybe"},
		{PageSize: 10, Verdict: "Interesting"},
		{PageSize: 10, Verdict: collectorapi.VerdictInteresting, Unseen: true},
		{PageSize: 10, Verdict: collectorapi.VerdictInteresting, Unseen: true, Cursor: catalogCursor{Posted: 10, GalleryID: 1, Before: true}.encode()},
	} {
		if _, err := s.List(t.Context(), options); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("%+v: %v", options, err)
		}
	}
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, result, []int64{1})
}
