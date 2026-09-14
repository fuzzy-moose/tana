package catalog

import (
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func assertCatalogIDs(t *testing.T, result collectorapi.CatalogResult, want []int64) {
	t.Helper()
	got := make([]int64, 0, len(result.Items))
	for _, item := range result.Items {
		got = append(got, item.GalleryID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("gallery IDs = %v, want %v", got, want)
	}
}

func TestCatalogCursorSurvivesNewGalleriesAndDeletedAnchor(t *testing.T) {
	db, service := openCatalog(t)
	for id := int64(1); id <= 5; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id / 2, Category: "Manga"}, true)
	}
	retain(t, db, panda.Metadata{ID: 8, Posted: 4, Category: "Manga", Tags: []string{"language:translated"}}, true)
	options := collectorapi.CatalogOptions{Query: "-language:translated", Categories: []string{"manga", "doujinshi"}, PageSize: 2}
	first, err := service.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, first, []int64{5, 4})
	retain(t, db, panda.Metadata{ID: 6, Posted: 3, Category: "Doujinshi"}, true)
	if _, err := db.Exec("DELETE FROM gallery_metadata WHERE gallery_id = 4"); err != nil {
		t.Fatal(err)
	}
	options.Cursor = first.NextCursor
	second, err := service.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, second, []int64{3, 2})
	options.Cursor = second.PreviousCursor
	previous, err := service.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, previous, []int64{6, 5})
	if previous.PreviousCursor != "" {
		t.Fatal("previous page passed the start")
	}
	// A saved link can outlive every matching gallery beyond its anchor.
	if _, err := db.Exec("DELETE FROM gallery_metadata WHERE gallery_id < 4"); err != nil {
		t.Fatal(err)
	}
	options.Cursor = first.NextCursor
	empty, err := service.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Items) != 0 || empty.NextCursor != "" || empty.PreviousCursor == "" {
		t.Fatalf("empty page must offer a way back: %+v", empty)
	}
	options.Cursor = empty.PreviousCursor
	previous, err = service.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, previous, []int64{6, 5})
}

func TestCatalogRejectsInvalidCursors(t *testing.T) {
	_, service := openCatalog(t)
	for _, value := range []string{"not-base64!", strings.Repeat("a", 257),
		base64.RawURLEncoding.EncodeToString([]byte("null")),
		base64.RawURLEncoding.EncodeToString([]byte(`{"gallery_id":-1}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"gallery_id":1,"posted":"invalid"}`)),
	} {
		if _, err := service.List(t.Context(), collectorapi.CatalogOptions{Cursor: value, PageSize: 24}); !errors.Is(err, ErrInvalidPagination) {
			t.Errorf("cursor %q: %v", value, err)
		}
	}
}

func TestCatalogEmptyPageRecoveryIncludesAnchor(t *testing.T) {
	for _, before := range []bool{false, true} {
		db, service := openCatalog(t)
		for id := int64(1); id <= 3; id++ {
			retain(t, db, panda.Metadata{ID: id, Posted: id}, true)
		}
		first, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 1, Cursor: first.NextCursor})
		if err != nil {
			t.Fatal(err)
		}
		cursor, removed := second.NextCursor, 1
		if before {
			cursor, removed = second.PreviousCursor, 3
		}
		if _, err := db.Exec("DELETE FROM gallery_metadata WHERE gallery_id = ?", removed); err != nil {
			t.Fatal(err)
		}
		empty, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 1, Cursor: cursor})
		if err != nil || len(empty.Items) != 0 {
			t.Fatalf("empty page: %+v, %v", empty, err)
		}
		cursor = empty.PreviousCursor
		if before {
			cursor = empty.NextCursor
		}
		if cursor == "" {
			t.Fatal("missing recovery cursor")
		}
		recovered, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 1, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		assertCatalogIDs(t, recovered, []int64{2})
		if (before && recovered.PreviousCursor != "") || (!before && recovered.NextCursor != "") {
			t.Fatalf("recovery links back to the empty page: %+v", recovered)
		}
	}
}
