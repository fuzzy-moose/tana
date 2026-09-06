package favorites

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
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
				if err := s.complete(t.Context(), 2, "Original", true, fixtureEntries(1, 100, 1000)); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, entry := range fixtureEntries(1, 100, 1000) {
					if _, err := s.db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?)`, entry.GalleryRef.ID, entry.GalleryRef.Token); err != nil {
						t.Fatal(err)
					}
				}
			}
			calls := 0
			client := pageFunc(func(_ context.Context, category int, next string) (panda.FavoritesPage, error) {
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
			})
			service := &Service{store: s, client: client}
			if err := service.collect(t.Context(), request{category: 2, full: tc.full}); err != nil {
				t.Fatal(err)
			}
			f, inventory := counts(t, s.db)
			if calls != tc.pages || f != tc.favorites || inventory != tc.inventory {
				t.Fatalf("pages=%d favorites=%d inventory=%d", calls, f, inventory)
			}
			var name string
			if err := s.db.QueryRow(`SELECT name FROM favorite_categories`).Scan(&name); err != nil || name != "Renamed" {
				t.Fatalf("name=%s, %v", name, err)
			}
		})
	}
}

func TestFullResyncRemovesFavoritesButRetainsInventoryAndMetadata(t *testing.T) {
	s := testStore(t)
	if err := s.complete(t.Context(), 2, "Original", true, fixtureEntries(1, 2, 1000)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (2, '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{1, 0} {
		service := &Service{store: s, client: pageFunc(func(context.Context, int, string) (panda.FavoritesPage, error) {
			return panda.FavoritesPage{CategoryName: "Original", Entries: fixtureEntries(1, count, 1000)}, nil
		})}
		if err := service.collect(t.Context(), request{category: 2, full: true}); err != nil {
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
	_, initialized, err := s.known(t.Context(), 2)
	if err != nil || !initialized {
		t.Fatalf("empty category lost sync state: %v", err)
	}
}

func TestFailedTraversalAndCommitPreserveSnapshot(t *testing.T) {
	s := testStore(t)
	if err := s.complete(t.Context(), 2, "Original", true, fixtureEntries(1, 1, 1000)); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: s, client: pageFunc(func(_ context.Context, _ int, next string) (panda.FavoritesPage, error) {
		if next != "" {
			return panda.FavoritesPage{}, panda.ErrFavoritesPage
		}
		return panda.FavoritesPage{CategoryName: "Changed", Entries: fixtureEntries(100, 100, 2000), Next: "second"}, nil
	})}
	if err := service.collect(t.Context(), request{category: 2, full: true}); !errors.Is(err, panda.ErrFavoritesPage) {
		t.Fatalf("error=%v", err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_favorite BEFORE INSERT ON favorites WHEN NEW.gallery_id = 999 BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.complete(t.Context(), 2, "Changed", true, fixtureEntries(999, 1, 2000)); err == nil {
		t.Fatal("expected commit failure")
	}
	f, inventory := counts(t, s.db)
	if f != 1 || inventory != 1 {
		t.Fatalf("partial changes committed: favorites=%d inventory=%d", f, inventory)
	}
	var name string
	if err := s.db.QueryRow(`SELECT name FROM favorite_categories`).Scan(&name); err != nil || name != "Original" {
		t.Fatalf("name=%s, %v", name, err)
	}
}

func TestCategoryStateIsScoped(t *testing.T) {
	s := testStore(t)
	if err := s.complete(t.Context(), 2, "Original", true, fixtureEntries(1, 1, 1000)); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct {
		host, member string
		category     int
	}{
		{s.host, s.accountKey, 3}, {s.host, "43", 2}, {"https://other.test", s.accountKey, 2},
	} {
		other := *s
		other.host, other.accountKey = scope.host, scope.member
		if _, initialized, err := other.known(t.Context(), scope.category); err != nil || initialized {
			t.Fatalf("state leaked across scope: %v", err)
		}
	}
}

func TestQueueCoalescesAndUpgrades(t *testing.T) {
	s := &Service{ctx: t.Context(), wake: make(chan struct{}, 1)}
	for _, full := range []bool{false, true, false} {
		if err := s.Enqueue(2, full); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.pending) != 1 || !s.pending[0].full {
		t.Fatalf("queued=%+v", s.pending)
	}
	s.pending = nil
	s.active = &request{category: 2}
	if err := s.Enqueue(2, false); err != nil {
		t.Fatal(err)
	}
	if len(s.pending) != 0 {
		t.Fatal("duplicated active job")
	}
	if err := s.Enqueue(2, true); err != nil {
		t.Fatal(err)
	}
	if len(s.pending) != 1 || !s.pending[0].full {
		t.Fatal("lost full re-sync behind active incremental run")
	}
}

func TestWorkerOnlyCollectsOnDemandAndStopsOnShutdown(t *testing.T) {
	s := testStore(t)
	called := make(chan int, 1)
	client := pageFunc(func(ctx context.Context, category int, next string) (panda.FavoritesPage, error) {
		called <- category
		<-ctx.Done()
		return panda.FavoritesPage{}, ctx.Err()
	})
	service := New(t.Context(), s.db, panda.FavoritesConfig{URL: s.host + "/account/saved-items?view=table", AccountKey: s.accountKey}, client, slog.New(slog.DiscardHandler))
	t.Cleanup(service.Close)
	if service.store.host != s.host {
		t.Fatal("favorites URL path or query changed collection identity")
	}
	if err := service.Enqueue(2, false); err != nil {
		t.Fatal(err)
	}
	select {
	case category := <-called:
		if category != 2 {
			t.Fatalf("category %d", category)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("accepted job never started")
	}
	service.Close()
	if f, inventory := counts(t, s.db); f != 0 || inventory != 0 {
		t.Fatal("shutdown committed unfinished work")
	}
	if err := service.Enqueue(2, false); !errors.Is(err, ErrClosed) {
		t.Fatalf("admitted work after shutdown: %v", err)
	}
}
