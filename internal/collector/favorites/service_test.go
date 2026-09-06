package favorites

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type pageFunc func(context.Context, int, string) (panda.FavoritesPage, error)

func (f pageFunc) GetFavoritesPage(ctx context.Context, category int, next string) (panda.FavoritesPage, error) {
	return f(ctx, category, next)
}

func fixtureEntries(first, count int, at int64) []panda.Favorite {
	entries := make([]panda.Favorite, count)
	for i := range entries {
		entries[i] = panda.Favorite{GalleryRef: panda.GalleryRef{ID: int64(first + i), Token: "123456789a"}, AddedAt: time.Unix(at, 0)}
	}
	return entries
}

func testStore(t *testing.T) *store {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &store{db: db, q: dbgen.New(db), host: "https://panda.test", accountKey: "42"}
}

func counts(t *testing.T, db *sql.DB) (int, int) {
	t.Helper()
	var favorites, inventory int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM favorites), (SELECT count(*) FROM gallery_refs)`).Scan(&favorites, &inventory); err != nil {
		t.Fatal(err)
	}
	return favorites, inventory
}

func admit(t *testing.T, s *store, category int, full bool) job {
	t.Helper()
	if err := s.enqueue(t.Context(), []int{category}, full); err != nil {
		t.Fatal(err)
	}
	current, err := s.next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func seed(t *testing.T, s *store, entries []panda.Favorite) {
	t.Helper()
	current := admit(t, s, 2, true)
	if err := s.savePage(t.Context(), current, panda.FavoritesPage{CategoryName: "Original", Entries: entries}, true, 0); err != nil {
		t.Fatal(err)
	}
}

func drain(t *testing.T, service *Service) error {
	t.Helper()
	for range 20 {
		current, err := service.store.next(t.Context())
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := service.collect(t.Context(), current); err != nil {
			return err
		}
	}
	t.Fatal("traversal did not finish")
	return nil
}

func categoryStatus(t *testing.T, s *store) collectorapi.FavoriteCategory {
	t.Helper()
	status, err := (&Service{store: s}).Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return status.Categories[2]
}

func TestTraversalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		initialized, full, refavorited bool
		pages, favorites, inventory    int
	}{
		{"initial ignores existing inventory", false, false, false, 2, 101, 101},
		{"incremental stops at known page", true, false, false, 1, 100, 100},
		{"new timestamps count as new", true, false, true, 2, 101, 101},
		{"full reads through known page", true, true, false, 2, 101, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			if tc.initialized {
				seed(t, s, fixtureEntries(1, 100, 1000))
			} else {
				for _, entry := range fixtureEntries(1, 100, 1000) {
					if err := s.q.SaveGalleryRef(t.Context(), dbgen.SaveGalleryRefParams{GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token}); err != nil {
						t.Fatal(err)
					}
				}
			}
			calls := 0
			service := &Service{store: s, client: pageFunc(func(_ context.Context, category int, next string) (panda.FavoritesPage, error) {
				calls++
				if category != 2 {
					t.Fatalf("category %d", category)
				}
				if next == "" {
					at := int64(1000)
					if tc.refavorited {
						at = 2000
					}
					return panda.FavoritesPage{CategoryName: "Renamed", Entries: fixtureEntries(1, 100, at), Next: "second"}, nil
				}
				return panda.FavoritesPage{CategoryName: "Renamed", Entries: fixtureEntries(101, 1, 500)}, nil
			})}
			admit(t, s, 2, tc.full)
			if err := drain(t, service); err != nil {
				t.Fatal(err)
			}
			f, inventory := counts(t, s.db)
			status := categoryStatus(t, s)
			if calls != tc.pages || f != tc.favorites || inventory != tc.inventory || status.PagesSaved != int64(calls) || status.LastSavedAt == nil || status.Name != "Renamed" {
				t.Fatalf("pages=%d favorites=%d inventory=%d status=%+v", calls, f, inventory, status)
			}
		})
	}
}

func TestFullResyncRemovesFavoritesButRetainsInventoryAndMetadata(t *testing.T) {
	s := testStore(t)
	seed(t, s, fixtureEntries(1, 2, 1000))
	if _, err := s.db.Exec(`INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (2, '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{1, 0} {
		admit(t, s, 2, true)
		service := &Service{store: s, client: pageFunc(func(context.Context, int, string) (panda.FavoritesPage, error) {
			return panda.FavoritesPage{CategoryName: "Original", Entries: fixtureEntries(1, count, 1000)}, nil
		})}
		if err := drain(t, service); err != nil {
			t.Fatal(err)
		}
		f, inventory := counts(t, s.db)
		if f != count || inventory != 2 {
			t.Fatalf("favorites=%d inventory=%d", f, inventory)
		}
	}
	var metadata int
	if err := s.db.QueryRow(`SELECT count(*) FROM gallery_metadata`).Scan(&metadata); err != nil || metadata != 1 {
		t.Fatalf("metadata=%d, %v", metadata, err)
	}
	if _, initialized, err := s.known(t.Context(), 2); err != nil || !initialized {
		t.Fatalf("empty category lost sync state: %v", err)
	}
}

func TestPageSavePublishesDiscoveriesWithoutAdvancingBoundary(t *testing.T) {
	s := testStore(t)
	seed(t, s, fixtureEntries(1, 1, 1000))
	before := categoryStatus(t, s)
	current := admit(t, s, 2, true)
	page := panda.FavoritesPage{CategoryName: "Changed", Entries: fixtureEntries(100, 100, 2000), Next: "second"}
	if err := s.savePage(t.Context(), current, page, false, 2000); err != nil {
		t.Fatal(err)
	}
	current, err := s.next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.failed(t.Context(), current, panda.ErrFavoritesPage); err != nil {
		t.Fatal(err)
	}
	f, inventory := counts(t, s.db)
	known, _, err := s.known(t.Context(), 2)
	status := categoryStatus(t, s)
	if f != 101 || inventory != 101 || err != nil || len(known) != 1 || known[1] != 1000 || !status.LastSyncedAt.Equal(*before.LastSyncedAt) || status.LastOutcome != "failed" || status.EntriesSaved != 100 {
		t.Fatalf("favorites=%d inventory=%d known=%v status=%+v error=%v", f, inventory, known, status, err)
	}
	// A rejected page rolls back its inventory, membership and checkpoint together.
	if _, err := s.db.Exec(`CREATE TRIGGER fail_favorite BEFORE INSERT ON favorites WHEN NEW.gallery_id = 999 BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	page = panda.FavoritesPage{CategoryName: "Wrong", Entries: fixtureEntries(999, 1, 1500)}
	if err := s.savePage(t.Context(), current, page, true, 1500); err == nil {
		t.Fatal("expected page save failure")
	}
	after := categoryStatus(t, s)
	if f2, i2 := counts(t, s.db); f2 != f || i2 != inventory || !reflect.DeepEqual(after, status) {
		t.Fatalf("failed page changed durable state: %+v", after)
	}
}

func TestRestartAutomaticallyResumesSavedCursorAndQueue(t *testing.T) {
	dir := t.TempDir()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &store{db: db, q: dbgen.New(db), host: "https://panda.test", accountKey: "42"}
	cfg := panda.FavoritesConfig{URL: s.host + "/favorites", AccountKey: s.accountKey}
	second := make(chan struct{})
	first := New(t.Context(), db, cfg, pageFunc(func(ctx context.Context, category int, next string) (panda.FavoritesPage, error) {
		if next == "" {
			return panda.FavoritesPage{CategoryName: "Manga", Entries: fixtureEntries(1, 100, 2000), Next: "second"}, nil
		}
		close(second)
		<-ctx.Done()
		return panda.FavoritesPage{}, ctx.Err()
	}), slog.New(slog.DiscardHandler))
	t.Cleanup(first.Close)
	if err := first.Enqueue(2, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("second page never requested")
	}
	if err := first.Enqueue(3, false); err != nil {
		t.Fatal(err)
	}
	first.Close()
	before := categoryStatus(t, s)
	if f, i := counts(t, db); f != 100 || i != 100 || before.PagesSaved != 1 || before.LastSyncedAt != nil {
		t.Fatalf("partial save missing: %+v, %d/%d", before, f, i)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.db, s.q = db, dbgen.New(db)
	requested := make(chan string, 2)
	resumed := New(t.Context(), db, cfg, pageFunc(func(_ context.Context, category int, next string) (panda.FavoritesPage, error) {
		requested <- next
		return panda.FavoritesPage{CategoryName: "Manga", Entries: fixtureEntries(101, 1, 1000)}, nil
	}), slog.New(slog.DiscardHandler))
	defer resumed.Close()
	for _, want := range []string{"second", ""} {
		select {
		case got := <-requested:
			if got != want {
				t.Fatalf("cursor=%q want=%q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("retained job not resumed")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for categoryStatus(t, s).LastOutcome != "success" {
		if time.Now().After(deadline) {
			t.Fatal("resume did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	after := categoryStatus(t, s)
	if after.EntriesSaved != 101 || after.PagesSaved != 2 || after.LastSyncedAt == nil || !after.StartedAt.Equal(*before.StartedAt) {
		t.Fatalf("resumed progress=%+v", after)
	}
}

func TestInvalidCursorRestartsWithOriginalIncrementalBoundary(t *testing.T) {
	s := testStore(t)
	seed(t, s, fixtureEntries(1, 1, 500))
	admit(t, s, 2, false)
	var calls []string
	failedOnce := false
	service := &Service{store: s, client: pageFunc(func(_ context.Context, _ int, next string) (panda.FavoritesPage, error) {
		calls = append(calls, next)
		switch next {
		case "":
			return panda.FavoritesPage{CategoryName: "Manga", Entries: fixtureEntries(1, 100, 2000), Next: "second"}, nil
		case "second":
			if !failedOnce {
				failedOnce = true
				return panda.FavoritesPage{}, &panda.HTTPError{StatusCode: http.StatusGone}
			}
			// The page overlaps one saved entry; discovery counts remain distinct.
			return panda.FavoritesPage{CategoryName: "Manga", Entries: fixtureEntries(100, 100, 1000), Next: "third"}, nil
		default:
			return panda.FavoritesPage{CategoryName: "Manga", Entries: fixtureEntries(500, 1, 500)}, nil
		}
	})}
	if err := drain(t, service); err != nil {
		t.Fatal(err)
	}
	want := []string{"", "second", "", "second", "third"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
	if f, i := counts(t, s.db); f != 200 || i != 200 {
		t.Fatalf("favorites=%d inventory=%d", f, i)
	}
	if got := categoryStatus(t, s).EntriesSaved; got != 200 {
		t.Fatalf("saved entries=%d", got)
	}
}

func TestInvalidCursorFallbackIsBounded(t *testing.T) {
	s := testStore(t)
	admit(t, s, 2, true)
	calls := 0
	service := &Service{store: s, client: pageFunc(func(_ context.Context, _ int, next string) (panda.FavoritesPage, error) {
		calls++
		if next != "" {
			return panda.FavoritesPage{}, panda.ErrFavoritesPage
		}
		return panda.FavoritesPage{CategoryName: "Manga", Entries: fixtureEntries(1, 100, 1000), Next: "bad"}, nil
	})}
	if err := drain(t, service); !errors.Is(err, panda.ErrFavoritesPage) || calls != 4 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestQueueCoalescesUpgradesAndRetainsFollowup(t *testing.T) {
	s := testStore(t)
	seed(t, s, fixtureEntries(1, 1, 1000))
	for _, full := range []bool{false, true, false} {
		if err := s.enqueue(t.Context(), []int{2}, full); err != nil {
			t.Fatal(err)
		}
	}
	current, err := s.next(t.Context())
	if err != nil || current.Full != 1 {
		t.Fatalf("queued=%+v %v", current, err)
	}
	if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: fixtureEntries(1, 1, 1000)}, true, 1000); err != nil {
		t.Fatal(err)
	}
	current = admit(t, s, 2, false)
	for _, full := range []bool{false, true, true} {
		if err := s.enqueue(t.Context(), []int{2}, full); err != nil {
			t.Fatal(err)
		}
	}
	if status := categoryStatus(t, s); !status.Queued || !status.QueuedFull || status.Full {
		t.Fatalf("followup=%+v", status)
	}
	if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: fixtureEntries(1, 1, 1000)}, true, 1000); err != nil {
		t.Fatal(err)
	}
	next, err := s.next(t.Context())
	if err != nil || next.Full != 1 || next.PagesSaved != 0 || next.FollowupFull != 0 {
		t.Fatalf("next=%+v %v", next, err)
	}
	service := &Service{store: s, ctx: t.Context(), wake: make(chan struct{}, 1)}
	if err := service.EnqueueAll(true); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range status.Categories {
		if category.Category != 2 && (!category.Queued || !category.QueuedFull) {
			t.Fatalf("category not queued: %+v", category)
		}
	}
}

func TestCooldownAndFailureSurviveRestart(t *testing.T) {
	for _, failure := range []error{panda.ErrFavoritesPage, &panda.BanError{Until: time.Now().Add(time.Hour)}} {
		t.Run(failure.Error(), func(t *testing.T) {
			s := testStore(t)
			current := admit(t, s, 2, true)
			if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: fixtureEntries(1, 100, 1000), Next: "second"}, false, 1000); err != nil {
				t.Fatal(err)
			}
			if err := s.failed(t.Context(), current, failure); err != nil {
				t.Fatal(err)
			}
			before := categoryStatus(t, s)
			called := make(chan struct{}, 1)
			service := New(t.Context(), s.db, panda.FavoritesConfig{URL: s.host, AccountKey: s.accountKey}, pageFunc(func(context.Context, int, string) (panda.FavoritesPage, error) {
				called <- struct{}{}
				return panda.FavoritesPage{}, failure
			}), slog.New(slog.DiscardHandler))
			// Persisted failures stay failed; cooldowns must prevent immediate retries.
			select {
			case <-called:
				t.Error("retried failed/cooling job on startup")
			case <-time.After(20 * time.Millisecond):
			}
			service.Close()
			after := categoryStatus(t, s)
			if !reflect.DeepEqual(before, after) || after.LastError == "" || after.EntriesSaved != 100 {
				t.Fatalf("before=%+v after=%+v", before, after)
			}
			if err := service.Enqueue(2, false); !errors.Is(err, ErrClosed) {
				t.Fatalf("admitted after shutdown: %v", err)
			}
		})
	}
}

func TestCategoryStateIsScoped(t *testing.T) {
	s := testStore(t)
	seed(t, s, fixtureEntries(1, 1, 1000))
	admit(t, s, 2, false)
	for _, scope := range []struct {
		host, account string
		category      int
	}{
		{s.host, s.accountKey, 3}, {s.host, "43", 2}, {"https://other.test", s.accountKey, 2},
	} {
		other := *s
		other.host, other.accountKey = scope.host, scope.account
		if _, initialized, err := other.known(t.Context(), scope.category); err != nil || initialized {
			t.Fatalf("state leaked across scope: %v", err)
		}
		if scope.category == 2 {
			if _, err := other.next(t.Context()); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("job leaked across scope: %v", err)
			}
		}
	}
}
