package favorites

import (
	"fmt"
	"maps"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// project gives gallery id an entry with tags in the Panda catalog.
func project(t *testing.T, s *store, id int64, tags ...string) {
	t.Helper()
	tx, err := s.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, '123456789a') ON CONFLICT DO NOTHING", id); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: fmt.Sprintf("Gallery %d", id), Posted: id, Tags: tags}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// favoriteEntries are the favorites of the given galleries.
func favoriteEntries(ids ...int64) []panda.Favorite {
	entries := make([]panda.Favorite, len(ids))
	for i, id := range ids {
		entries[i] = fixtureEntries(int(id), 1, 1000)[0]
	}
	return entries
}

// completeSync runs one sync of category whose one page holds entries.
func completeSync(t *testing.T, s *store, category int, full bool, entries []panda.Favorite) {
	t.Helper()
	current := admit(t, s, category, full)
	if current.Full != full {
		t.Fatalf("sync full = %v, want %v", current.Full, full)
	}
	if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: entries}, true, nil); err != nil {
		t.Fatal(err)
	}
}

func assertAuthors(t *testing.T, s *store, want map[string]int) {
	t.Helper()
	rows, err := s.db.Query(t.Context(), `SELECT t.namespace || ':' || t.value, a.favorites
		FROM favorite_authors a JOIN catalog_tags t ON t.id = a.tag_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var tag string
		var favorites int
		if err := rows.Scan(&tag, &favorites); err != nil {
			t.Fatal(err)
		}
		got[tag] = favorites
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Fatalf("favorite authors = %v, want %v", got, want)
	}
}

func TestFavoriteAuthorsAreArtistTagsOfTwoFavorites(t *testing.T) {
	s := testStore(t)
	other := &store{db: s.db, q: s.q, host: s.host, accountKey: "43"}
	project(t, s, 1, "artist:a", "artist:b", "group:g")
	project(t, s, 2, "artist:a", "group:g", "parody:p")
	project(t, s, 3, "artist:c", "parody:p")
	project(t, s, 4, "artist:a", "artist:c")
	completeSync(t, s, 2, true, favoriteEntries(1, 2))
	completeSync(t, s, 3, true, favoriteEntries(2))
	completeSync(t, other, 2, true, favoriteEntries(3, 4))
	assertAuthors(t, s, map[string]int{"artist:a": 3, "artist:c": 2})
}

func TestFavoriteAuthorsCountFavoritesWithCatalogTags(t *testing.T) {
	s := testStore(t)
	project(t, s, 1, "artist:a")
	project(t, s, 2, "artist:a")
	completeSync(t, s, 2, true, favoriteEntries(1, 2, 3))
	assertAuthors(t, s, map[string]int{"artist:a": 2})
	project(t, s, 3, "artist:a")
	assertAuthors(t, s, map[string]int{"artist:a": 2})
	completeSync(t, s, 2, false, nil)
	assertAuthors(t, s, map[string]int{"artist:a": 3})
}

func TestOnlyFullResyncRemovesFavoriteAuthors(t *testing.T) {
	s := testStore(t)
	project(t, s, 1, "artist:a", "artist:b", "artist:e")
	project(t, s, 2, "artist:b", "artist:e")
	project(t, s, 5, "artist:d")
	project(t, s, 6, "artist:d")
	completeSync(t, s, 2, true, favoriteEntries(1, 2))
	completeSync(t, s, 3, true, favoriteEntries(5, 6))
	assertAuthors(t, s, map[string]int{"artist:b": 2, "artist:e": 2, "artist:d": 2})

	// Gallery 2 loses artist:b in the catalog; an incremental sync adds and raises authors.
	project(t, s, 2, "artist:e")
	project(t, s, 3, "artist:a", "artist:c")
	project(t, s, 4, "artist:c", "artist:d")
	completeSync(t, s, 2, false, favoriteEntries(3, 4))
	assertAuthors(t, s, map[string]int{"artist:a": 2, "artist:b": 2, "artist:c": 2, "artist:d": 3, "artist:e": 2})

	// Gallery 2 is gone upstream: the full re-sync removes artist:b and artist:e
	// and keeps artist:d, whose favorites 5 and 6 are in another collection.
	completeSync(t, s, 2, true, favoriteEntries(1, 3, 4))
	assertAuthors(t, s, map[string]int{"artist:a": 2, "artist:c": 2, "artist:d": 3})
}

func TestUnfinishedSyncChangesNoFavoriteAuthors(t *testing.T) {
	s := testStore(t)
	project(t, s, 1, "artist:a", "artist:b")
	project(t, s, 2, "artist:a")
	project(t, s, 3, "artist:a", "artist:b")
	completeSync(t, s, 2, true, favoriteEntries(1, 2))
	assertAuthors(t, s, map[string]int{"artist:a": 2})

	current := admit(t, s, 2, false)
	if err := s.savePage(t.Context(), current, panda.FavoritesPage{Entries: favoriteEntries(3), Next: "older"}, false, nil); err != nil {
		t.Fatal(err)
	}
	assertAuthors(t, s, map[string]int{"artist:a": 2})
	if err := s.failed(t.Context(), current, fmt.Errorf("broken profile: %w", panda.ErrFavoritesPage)); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.db.QueryRow(t.Context(), "SELECT state FROM favorite_syncs WHERE category_id = $1", current.CategoryID).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("sync state = %q %v", state, err)
	}
	assertAuthors(t, s, map[string]int{"artist:a": 2})
}
