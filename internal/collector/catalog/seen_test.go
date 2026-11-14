package catalog

import (
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func displayTimes(t *testing.T, db *pgxpool.Pool, id int64) []time.Time {
	t.Helper()
	rows, err := db.Query(t.Context(), "SELECT displayed_at FROM panda_gallery_displays WHERE gallery_id = $1 ORDER BY displayed_at", id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var times []time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			t.Fatal(err)
		}
		times = append(times, at)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return times
}

func displayedIDs(t *testing.T, s *Service, options collectorapi.CatalogOptions) map[int64]bool {
	t.Helper()
	options.PageSize = 100
	result, err := s.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	displayed := make(map[int64]bool, len(result.Items))
	for _, item := range result.Items {
		displayed[item.GalleryID] = item.Displayed
	}
	return displayed
}

func TestRecordDisplaysKeepsEachRecord(t *testing.T) {
	db, s := openCatalog(t)
	if err := s.RecordDisplays(t.Context(), []int64{3, 1, 3}, ""); err != nil {
		t.Fatal(err)
	}
	first := displayTimes(t, db, 3)
	if len(first) != 1 || len(displayTimes(t, db, 1)) != 1 {
		t.Fatalf("displays after first record: 3=%v 1=%v", first, displayTimes(t, db, 1))
	}
	if err := s.RecordDisplays(t.Context(), []int64{3}, ""); err != nil {
		t.Fatal(err)
	}
	second := displayTimes(t, db, 3)
	if len(second) != 2 || !second[0].Equal(first[0]) || !second[1].After(first[0]) {
		t.Fatalf("displays after second record: %v", second)
	}
}

func TestRecordDisplaysRejectsInvalidLists(t *testing.T) {
	db, s := openCatalog(t)
	tooMany := make([]int64, collectorapi.MaxCatalogDisplays+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for _, ids := range [][]int64{nil, {}, tooMany, {0}, {-1}, {2, 0, 5}} {
		if err := s.RecordDisplays(t.Context(), ids, ""); !errors.Is(err, ErrInvalidDisplays) {
			t.Fatalf("%v: %v", ids, err)
		}
	}
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_gallery_displays").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid records stored %d displays, %v", count, err)
	}
}

func TestCatalogFlagsDisplayedGalleries(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "Red one", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "Red two", Posted: 20}, true)
	retain(t, db, panda.Metadata{ID: 3, Title: "Blue", Posted: 30}, true)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 4, Token: "token4"}, Title: "Red feed"}, time.Unix(40, 0))
	if err := s.RecordDisplays(t.Context(), []int64{2, 4}, ""); err != nil {
		t.Fatal(err)
	}
	want := map[int64]bool{1: false, 2: true, 3: false, 4: true}
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{}); !maps.Equal(got, want) {
		t.Fatalf("all galleries: %v", got)
	}
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{Query: "red"}); !maps.Equal(got, map[int64]bool{1: false, 2: true, 4: true}) {
		t.Fatalf("search: %v", got)
	}
	// Collected metadata replaces the provisional row the display was recorded on.
	retain(t, db, panda.Metadata{ID: 4, Title: "Red collected", Posted: 40}, true)
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{}); !maps.Equal(got, want) {
		t.Fatalf("after collection: %v", got)
	}
	if _, err := db.Exec(t.Context(), "DELETE FROM catalog_galleries WHERE gallery_id = 4"); err != nil {
		t.Fatal(err)
	}
	retain(t, db, panda.Metadata{ID: 4, Title: "Red collected", Posted: 40}, true)
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{}); !maps.Equal(got, want) {
		t.Fatalf("after re-projection: %v", got)
	}
	s = New(pgtest.Reopen(t, db, storage.Migrations()), "https://favorites.example.test")
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{}); !maps.Equal(got, want) {
		t.Fatalf("after reopen: %v", got)
	}
}

func unseenPage(t *testing.T, s *Service, options collectorapi.CatalogOptions) collectorapi.CatalogResult {
	t.Helper()
	options.Unseen = true
	result, err := s.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousCursor != "" {
		t.Fatalf("unseen page has a previous cursor: %+v", result)
	}
	if (len(result.Items) > 0) != (result.NextCursor != "") {
		t.Fatalf("unseen page next cursor = %q with %d items", result.NextCursor, len(result.Items))
	}
	return result
}

func TestUnseenViewLeavesOutDisplayedGalleriesUnderEveryFilter(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Category: "Manga", Title: "red one", Posted: 10},
		{ID: 2, Category: "Doujinshi", Title: "red two", Posted: 20},
		{ID: 3, Category: "Manga", Title: "blue three", Posted: 30},
		{ID: 4, Category: "Artist CG", Title: "red four", Posted: 40, Expunged: true},
		{ID: 5, Category: "Doujinshi", Title: "blue five", Posted: 50},
		{ID: 6, Category: "Manga", Title: "red six", Posted: 50},
		{ID: 7, Category: "Doujinshi", Title: "red seven", Posted: 60, Expunged: true},
		{ID: 8, Category: "Manga", Title: "blue eight", Posted: 70},
	} {
		retain(t, db, entry, true)
	}
	if err := s.RecordDisplays(t.Context(), []int64{2, 6, 7, 8}, ""); err != nil {
		t.Fatal(err)
	}
	for name, options := range map[string]collectorapi.CatalogOptions{
		"all":                                {},
		"query":                              {Query: "red"},
		"one category":                       {Categories: []string{"manga"}},
		"two categories":                     {Categories: []string{"manga", "doujinshi"}},
		"defaults":                           {DefaultQuery: "-blue", DefaultCategories: []string{"manga", "doujinshi", "artist cg"}},
		"expunged":                           {IncludeExpunged: true},
		"expunged two categories with query": {Query: "red", Categories: []string{"doujinshi", "artist cg"}, IncludeExpunged: true},
	} {
		t.Run(name, func(t *testing.T) {
			options.PageSize = 100
			all, err := s.List(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			want := []int64{}
			for _, item := range all.Items {
				if !item.Displayed {
					want = append(want, item.GalleryID)
				}
			}
			if len(want) == len(all.Items) {
				t.Fatalf("fixture shows no displayed gallery: %v", want)
			}
			assertCatalogIDs(t, unseenPage(t, s, options), want)
		})
	}
}

func TestUnseenViewKeepsGalleriesHiddenFromARecordedPage(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "red one", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "blue two", Posted: 20}, true)
	retain(t, db, panda.Metadata{ID: 3, Title: "red three", Posted: 30}, true)
	searched := unseenPage(t, s, collectorapi.CatalogOptions{Query: "red", PageSize: 100})
	ids := []int64{}
	for _, item := range searched.Items {
		ids = append(ids, item.GalleryID)
	}
	if err := s.RecordDisplays(t.Context(), ids, ""); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{2})
	// A new version is its own gallery: recording gallery 1 leaves gallery 2 unseen.
	retain(t, db, panda.Metadata{ID: 4, Title: "blue two, new version", Posted: 40}, true)
	if err := s.RecordDisplays(t.Context(), []int64{2}, ""); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{4})
}

func TestUnseenPagingRunsForwardOnly(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 5; id++ {
		retain(t, db, panda.Metadata{ID: id, Title: "gallery", Posted: id * 10}, true)
	}
	options := collectorapi.CatalogOptions{PageSize: 2}
	first := unseenPage(t, s, options)
	assertCatalogIDs(t, first, []int64{5, 4})
	options.Cursor = first.NextCursor
	second := unseenPage(t, s, options)
	assertCatalogIDs(t, second, []int64{3, 2})
	options.Cursor = second.NextCursor
	last := unseenPage(t, s, options)
	assertCatalogIDs(t, last, []int64{1})
	options.Cursor = last.NextCursor
	assertCatalogIDs(t, unseenPage(t, s, options), []int64{})

	if err := s.RecordDisplays(t.Context(), []int64{3}, ""); err != nil {
		t.Fatal(err)
	}
	options.Cursor = first.NextCursor
	assertCatalogIDs(t, unseenPage(t, s, options), []int64{2, 1})
	if err := s.RecordDisplays(t.Context(), []int64{2, 1}, ""); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, unseenPage(t, s, options), []int64{})

	all, err := s.List(t.Context(), collectorapi.CatalogOptions{Cursor: first.NextCursor, PageSize: 2})
	if err != nil || all.PreviousCursor == "" {
		t.Fatalf("all galleries after first page: %+v, %v", all, err)
	}
	options.Cursor = all.PreviousCursor
	options.Unseen = true
	if _, err := s.List(t.Context(), options); !errors.Is(err, ErrInvalidPagination) {
		t.Fatalf("unseen with before cursor: %v", err)
	}
}

func TestUnseenGalleryNewerThanTheCursorWaitsForTheNewestPage(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 4; id++ {
		retain(t, db, panda.Metadata{ID: id, Title: "gallery", Posted: id * 10}, true)
	}
	first := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 2})
	assertCatalogIDs(t, first, []int64{4, 3})
	retain(t, db, panda.Metadata{ID: 5, Title: "gallery", Posted: 50}, true)
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{Cursor: first.NextCursor, PageSize: 2}), []int64{2, 1})
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 2}), []int64{5, 4})
}

type catchUpRow struct {
	orderedAt  time.Time
	galleryID  int64
	caughtUpAt time.Time
}

func storedCatchUp(t *testing.T, db *pgxpool.Pool) (catchUpRow, bool) {
	t.Helper()
	var row catchUpRow
	err := db.QueryRow(t.Context(), "SELECT ordered_at, gallery_id, caught_up_at FROM panda_catalog_catch_up").Scan(&row.orderedAt, &row.galleryID, &row.caughtUpAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return row, true
}

func catchUp(t *testing.T, s *Service, id int64) {
	t.Helper()
	if err := s.CatchUp(t.Context(), id); err != nil {
		t.Fatal(err)
	}
}

func TestCatchUpCoversGalleriesAtAndBeforeItsPosition(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Category: "Manga", Title: "red one", Posted: 10},
		{ID: 2, Category: "Doujinshi", Title: "blue two", Posted: 20},
		{ID: 3, Category: "Manga", Title: "red three", Posted: 30, Expunged: true},
		{ID: 4, Category: "Manga", Title: "red four", Posted: 40},
		{ID: 5, Category: "Doujinshi", Title: "blue five", Posted: 40},
		{ID: 6, Category: "Manga", Title: "red six", Posted: 50},
		{ID: 7, Category: "Doujinshi", Title: "red seven", Posted: 60, Expunged: true},
		{ID: 8, Category: "Manga", Title: "blue eight", Posted: 70},
	} {
		retain(t, db, entry, true)
	}
	if err := s.RecordDisplays(t.Context(), []int64{8}, ""); err != nil {
		t.Fatal(err)
	}
	catchUp(t, s, 4)
	stored, ok := storedCatchUp(t, db)
	if !ok || stored.galleryID != 4 || !stored.orderedAt.Equal(time.Unix(40, 0)) || stored.caughtUpAt.IsZero() {
		t.Fatalf("stored catch-up = %+v, %v", stored, ok)
	}
	// Gallery 5 shares gallery 4's time but has a greater ID, so it is newer.
	for name, tc := range map[string]struct {
		options collectorapi.CatalogOptions
		want    []int64
	}{
		"all":            {collectorapi.CatalogOptions{}, []int64{6, 5}},
		"query":          {collectorapi.CatalogOptions{Query: "red"}, []int64{6}},
		"one category":   {collectorapi.CatalogOptions{Categories: []string{"doujinshi"}}, []int64{5}},
		"two categories": {collectorapi.CatalogOptions{Categories: []string{"manga", "doujinshi"}}, []int64{6, 5}},
		"defaults":       {collectorapi.CatalogOptions{DefaultQuery: "-blue", DefaultCategories: []string{"manga", "doujinshi"}}, []int64{6}},
		"expunged":       {collectorapi.CatalogOptions{IncludeExpunged: true, Query: "red"}, []int64{7, 6}},
	} {
		t.Run(name, func(t *testing.T) {
			tc.options.PageSize = 100
			assertCatalogIDs(t, unseenPage(t, s, tc.options), tc.want)
		})
	}
	// Covered galleries keep the normal look, and the catch-up records no display.
	want := map[int64]bool{1: false, 2: false, 4: false, 5: false, 6: false, 8: true}
	if got := displayedIDs(t, s, collectorapi.CatalogOptions{}); !maps.Equal(got, want) {
		t.Fatalf("all galleries: %v", got)
	}
	var displays int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_gallery_displays").Scan(&displays); err != nil || displays != 1 {
		t.Fatalf("displays = %d, %v", displays, err)
	}
}

func TestCatchUpOnlyAdvances(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 6; id++ {
		retain(t, db, panda.Metadata{ID: id, Title: "gallery", Posted: id * 10}, true)
	}
	catchUp(t, s, 3)
	first, _ := storedCatchUp(t, db)
	catchUp(t, s, 2)
	catchUp(t, s, 3)
	if again, _ := storedCatchUp(t, db); again != first {
		t.Fatalf("catch-up that does not advance changed the row: %+v, was %+v", again, first)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{6, 5, 4})
	catchUp(t, s, 4)
	advanced, _ := storedCatchUp(t, db)
	if advanced.galleryID != 4 || !advanced.caughtUpAt.After(first.caughtUpAt) {
		t.Fatalf("advanced catch-up = %+v, was %+v", advanced, first)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{6, 5})
}

func TestConcurrentCatchUpsKeepTheNewestPosition(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 3; id++ {
		retain(t, db, panda.Metadata{ID: id, Title: "gallery", Posted: id * 10}, true)
	}
	for _, order := range [][2]int64{{3, 2}, {2, 3}} {
		if _, err := db.Exec(t.Context(), "DELETE FROM panda_catalog_catch_up"); err != nil {
			t.Fatal(err)
		}
		// An uncommitted catch-up at gallery 1 holds both calls until it commits.
		blocker, err := db.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := blocker.Exec(t.Context(), `INSERT INTO panda_catalog_catch_up (id, ordered_at, gallery_id, caught_up_at)
			VALUES (1, to_timestamp(10), 1, now())`); err != nil {
			t.Fatal(err)
		}
		errs := make(chan error, 2)
		for i, id := range order {
			go func() { errs <- s.CatchUp(t.Context(), id) }()
			waitForLockWaiters(t, db, i+1)
		}
		if err := blocker.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		for range order {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		if stored, _ := storedCatchUp(t, db); stored.galleryID != 3 {
			t.Fatalf("order %v stored gallery %d", order, stored.galleryID)
		}
	}
}

func waitForLockWaiters(t *testing.T, db *pgxpool.Pool, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := db.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d sessions wait for a lock, want %d", waiting, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCatchUpCoversGalleriesProjectedLaterAndSurvivesReprojection(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 10, Title: "anchor", Posted: 100}, true)
	retain(t, db, panda.Metadata{ID: 20, Title: "newer", Posted: 200}, true)
	catchUp(t, s, 10)
	retain(t, db, panda.Metadata{ID: 5, Title: "older collected", Posted: 50}, true)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 6, Token: "token6"}, Title: "older provisional"}, time.Unix(60, 0))
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 7, Token: "token7"}, Title: "newer provisional"}, time.Unix(150, 0))
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{20, 7})

	if _, err := db.Exec(t.Context(), "DELETE FROM catalog_galleries WHERE gallery_id = 10"); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{20, 7})
	retain(t, db, panda.Metadata{ID: 10, Title: "anchor", Posted: 100}, true)
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{20, 7})
}

func TestCatchUpRejectsUnknownGalleries(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Title: "gallery", Posted: 10}, true)
	retain(t, db, panda.Metadata{ID: 2, Title: "gallery", Posted: 20}, false)
	if err := s.CatchUp(t.Context(), 0); !errors.Is(err, ErrInvalidGalleryReference) {
		t.Fatalf("catch-up at 0: %v", err)
	}
	if err := s.CatchUp(t.Context(), 2); !errors.Is(err, ErrGalleryNotInCatalog) {
		t.Fatalf("catch-up without a catalog row: %v", err)
	}
	if _, ok := storedCatchUp(t, db); ok {
		t.Fatal("rejected catch-ups stored a position")
	}
	catchUp(t, s, 1)
	first, _ := storedCatchUp(t, db)
	if err := s.CatchUp(t.Context(), 99); !errors.Is(err, ErrGalleryNotInCatalog) {
		t.Fatalf("catch-up at unknown gallery: %v", err)
	}
	if again, _ := storedCatchUp(t, db); again != first {
		t.Fatalf("rejected catch-up changed the row: %+v", again)
	}
}
