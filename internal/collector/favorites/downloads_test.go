package favorites

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func manualService(t *testing.T, s *store, client PageClient) *Service {
	t.Helper()
	return &Service{store: s, ctx: t.Context(), wake: make(chan struct{}, 1), client: client}
}

func configureDownloads(t *testing.T, s *store, categories ...int) {
	t.Helper()
	if categories == nil {
		categories = []int{}
	}
	if err := (&Service{store: s}).SetDownloadSettings(t.Context(), collectorapi.FavoriteDownloadSettings{Categories: categories}); err != nil {
		t.Fatal(err)
	}
}

func downloadIDs(t *testing.T, s *store, want ...int64) {
	t.Helper()
	rows, err := s.db.Query(`SELECT gallery_id FROM panda_downloads ORDER BY gallery_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("downloads=%v want=%v", got, want)
	}
}

func bootstrap(t *testing.T, s *store) {
	t.Helper()
	service := manualService(t, s, pageFunc(func(_ context.Context, category int, _ string) (panda.FavoritesPage, error) {
		return panda.FavoritesPage{Entries: fixtureEntries(category+1, 1, 1000)}, nil
	}))
	if err := service.Enqueue(2, false); err != nil {
		t.Fatal(err)
	}
	if err := drain(t, service); err != nil {
		t.Fatal(err)
	}
}

func syncEntries(t *testing.T, s *store, category int, entries []panda.Favorite) {
	t.Helper()
	service := manualService(t, s, pageFunc(func(context.Context, int, string) (panda.FavoritesPage, error) {
		return panda.FavoritesPage{Entries: entries}, nil
	}))
	if err := service.Enqueue(category, true); err != nil {
		t.Fatal(err)
	}
	if err := drain(t, service); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadBootstrapRefreshesAllCategoriesOnExistingCollector(t *testing.T) {
	s := testStore(t)
	seed(t, s, fixtureEntries(1, 1, 1000))
	settings, err := (&Service{store: s}).DownloadSettings(t.Context())
	if err != nil || len(settings.Categories) != 0 {
		t.Fatalf("defaults=%+v %v", settings, err)
	}
	configureDownloads(t, s, 2)
	var calls [10]int
	service := manualService(t, s, pageFunc(func(_ context.Context, category int, next string) (panda.FavoritesPage, error) {
		calls[category]++
		if category == 2 && next == "" {
			return panda.FavoritesPage{Entries: fixtureEntries(1, 1, 1000), Next: "older"}, nil
		}
		return panda.FavoritesPage{Entries: fixtureEntries(10+category, 1, 500)}, nil
	}))
	if err := service.Enqueue(2, false); err != nil {
		t.Fatal(err)
	}
	if err := drain(t, service); err != nil {
		t.Fatal(err)
	}
	for category, count := range calls {
		want := 1
		if category == 2 {
			want = 2
		}
		if count != want {
			t.Fatalf("category %d fetched %d pages, want %d", category, count, want)
		}
	}
	status, err := service.Status(t.Context())
	if err != nil || status.Downloads.BaselineState != "ready" || status.Downloads.BaselineCategories != 10 {
		t.Fatalf("baseline=%+v %v", status.Downloads, err)
	}
	downloadIDs(t, s)
	// Changed timestamps, cross-category moves and older baseline entries stay known.
	syncEntries(t, s, 2, append(fixtureEntries(1, 1, 2000), append(fixtureEntries(10, 10, 1500), fixtureEntries(100, 1, 100)...)...))
	downloadIDs(t, s, 100)
}

func TestBootstrapWaitsForFreshTraversalAfterInFlightSync(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full=%t", full), func(t *testing.T) {
			s := testStore(t)
			seed(t, s, fixtureEntries(1, 1, 1000))
			current := admit(t, s, 2, full)
			configureDownloads(t, s, 2)
			service := manualService(t, s, pageFunc(func(_ context.Context, category int, _ string) (panda.FavoritesPage, error) {
				return panda.FavoritesPage{Entries: fixtureEntries(100+category, 1, 2000)}, nil
			}))
			if err := service.Enqueue(2, false); err != nil {
				t.Fatal(err)
			}
			// The old request returns after bootstrap admission. Its stale job
			// snapshot must not count as a completed baseline category.
			if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: fixtureEntries(1, 1, 1000)}, true, 1000); err != nil {
				t.Fatal(err)
			}
			status, err := service.Status(t.Context())
			if err != nil || status.Downloads.BaselineCategories != 0 {
				t.Fatalf("premature baseline: %+v %v", status.Downloads, err)
			}
			if err := drain(t, service); err != nil {
				t.Fatal(err)
			}
			status, err = service.Status(t.Context())
			if err != nil || status.Downloads.BaselineState != "ready" {
				t.Fatalf("unfinished baseline: %+v %v", status.Downloads, err)
			}
			downloadIDs(t, s)
			syncEntries(t, s, 2, fixtureEntries(102, 1, 3000))
			downloadIDs(t, s)
		})
	}
}

func TestDownloadEligibilitySurvivesSettingsChangesAndDeletion(t *testing.T) {
	s := testStore(t)
	jobs, err := downloads.New(t.Context(), s.db, t.TempDir(), nil, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	jobs.Close() // Exercise admission/cancellation/deletion without fetching archives.
	configureDownloads(t, s, 2)
	bootstrap(t, s)
	syncEntries(t, s, 2, fixtureEntries(100, 2, 2000))
	syncEntries(t, s, 3, fixtureEntries(200, 1, 2000))
	downloadIDs(t, s, 100, 101)
	configureDownloads(t, s, 2, 3)
	syncEntries(t, s, 3, fixtureEntries(200, 2, 3000))
	if _, err := jobs.Cancel(t.Context(), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.Cancel(t.Context(), 101); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Delete(t.Context(), 101); err != nil {
		t.Fatal(err)
	}
	syncEntries(t, s, 2, nil)
	syncEntries(t, s, 2, append(fixtureEntries(100, 2, 4000), fixtureEntries(200, 1, 3000)...))
	downloadIDs(t, s, 100, 201)
	job, err := jobs.Get(t.Context(), 100)
	if err != nil || job.State != "cancelled" {
		t.Fatalf("cancelled job=%+v %v", job, err)
	}
	configureDownloads(t, s)
	job, err = jobs.Get(t.Context(), 201)
	if err != nil || job.State != "queued" {
		t.Fatalf("queued job=%+v %v", job, err)
	}
	syncEntries(t, s, 2, fixtureEntries(300, 1, 5000))
	configureDownloads(t, s, 2)
	syncEntries(t, s, 2, fixtureEntries(300, 1, 5000))
	downloadIDs(t, s, 100, 201)
	// Explicit requests remain available after automatic eligibility is consumed.
	if _, err := jobs.Submit(t.Context(), fixtureEntries(101, 1, 1)[0].GalleryRef); err != nil {
		t.Fatal(err)
	}
	downloadIDs(t, s, 100, 101, 201)
}

func TestFavoriteDiscoveryAndDownloadCommitTogether(t *testing.T) {
	s := testStore(t)
	configureDownloads(t, s, 2)
	bootstrap(t, s)
	current := admit(t, s, 2, true)
	page := panda.FavoritesPage{Entries: fixtureEntries(500, 1, 2000), Next: "second"}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_download BEFORE INSERT ON panda_downloads BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.savePage(t.Context(), current, page, false, 2000); err == nil {
		t.Fatal("expected download admission failure")
	}
	var observed int
	if err := s.db.QueryRow(`SELECT count(*) FROM favorite_observations WHERE gallery_id = 500`).Scan(&observed); err != nil || observed != 0 {
		t.Fatalf("failed transaction consumed eligibility: %d %v", observed, err)
	}
	if categoryStatus(t, s).PagesSaved != 0 {
		t.Fatal("failed transaction advanced checkpoint")
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_download`); err != nil {
		t.Fatal(err)
	}
	if err := s.savePage(t.Context(), current, page, false, 2000); err != nil {
		t.Fatal(err)
	}
	if err := s.failed(t.Context(), current, panda.ErrFavoritesPage); err != nil {
		t.Fatal(err)
	}
	downloadIDs(t, s, 500)
	configureDownloads(t, s)
	// Restart retains both the download request and settings, despite a failed sync.
	configureDownloads(t, s, 3)
	var path string
	var seq int
	var name string
	if err := s.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err := storage.Open(t.Context(), filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.db, s.q = db, dbgen.New(db)
	downloadIDs(t, s, 500)
	settings, err := (&Service{store: s}).DownloadSettings(t.Context())
	if err != nil || !reflect.DeepEqual(settings.Categories, []int{3}) {
		t.Fatalf("settings=%+v %v", settings, err)
	}
}

func TestFailedBootstrapResumesOnRestartWithoutDownloading(t *testing.T) {
	s := testStore(t)
	configureDownloads(t, s, 0, 2)
	service := manualService(t, s, nil)
	if err := service.Enqueue(2, false); err != nil {
		t.Fatal(err)
	}
	current, err := s.next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: fixtureEntries(1, 1, 2000), Next: "second"}, false, 2000); err != nil {
		t.Fatal(err)
	}
	if err := s.failed(t.Context(), current, panda.ErrFavoritesPage); err != nil {
		t.Fatal(err)
	}
	// Finish the other categories; one failure keeps the entire baseline incomplete.
	service.client = pageFunc(func(context.Context, int, string) (panda.FavoritesPage, error) { return panda.FavoritesPage{}, nil })
	if err := drain(t, service); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(t.Context())
	if err != nil || status.Downloads.BaselineState != "collecting" || status.Downloads.BaselineCategories != 9 {
		t.Fatalf("baseline=%+v %v", status.Downloads, err)
	}
	requested := make(chan string, 1)
	resumed := New(t.Context(), s.db, panda.AuthenticatedConfig{FavoritesURL: s.host, AccountKey: s.accountKey}, pageFunc(func(_ context.Context, _ int, next string) (panda.FavoritesPage, error) {
		requested <- next
		return panda.FavoritesPage{Entries: fixtureEntries(2, 1, 1000)}, nil
	}), slog.New(slog.DiscardHandler))
	defer resumed.Close()
	select {
	case next := <-requested:
		if next != "second" {
			t.Fatalf("resume cursor=%q", next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed bootstrap did not resume")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, err = resumed.Status(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if status.Downloads.BaselineState == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bootstrap never completed")
		}
		time.Sleep(time.Millisecond)
	}
	resumed.Close()
	downloadIDs(t, s)
	if _, err := s.next(t.Context()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("bootstrap left work: %v", err)
	}
	syncEntries(t, s, 0, fixtureEntries(1, 3, 3000))
	downloadIDs(t, s, 3)
}
