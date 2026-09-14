package catalog

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestCatalogCategoryAndDefaultFilters(t *testing.T) {
	db, service := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Category: "Manga", Title: "red green", Tags: []string{"language:english"}},
		{ID: 2, Category: "Doujinshi", Title: "blue yellow", Tags: []string{"language:japanese"}},
		{ID: 3, Category: "Artist CG", Title: "red green"},
		{ID: 4, Category: "Manga", Title: "red"},
		{ID: 5, Category: "", Title: "red green"},
	} {
		retain(t, db, entry, true)
	}
	for _, tc := range []struct {
		name    string
		options collectorapi.CatalogOptions
		want    []int64
	}{
		{"all", collectorapi.CatalogOptions{}, []int64{5, 4, 3, 2, 1}},
		{"category union", collectorapi.CatalogOptions{Categories: []string{" manga ", "DOUJINSHI", "Manga"}}, []int64{4, 2, 1}},
		{"independent category groups", collectorapi.CatalogOptions{Categories: []string{"manga", "doujinshi"}, DefaultCategories: []string{"manga", "artist cg"}}, []int64{4, 1}},
		{"independent search alternatives", collectorapi.CatalogOptions{Query: "~red ~blue", DefaultQuery: "~green ~yellow"}, []int64{5, 3, 2, 1}},
		{"categories search and exclusions", collectorapi.CatalogOptions{Query: "~red ~blue", Categories: []string{"manga", "doujinshi"}, DefaultQuery: "-l:japanese$ ~green ~yellow"}, []int64{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.options.Page, tc.options.PageSize = 1, 100
			result, err := service.List(t.Context(), tc.options)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				got = append(got, item.GalleryID)
			}
			if !reflect.DeepEqual(got, tc.want) || result.Total != int64(len(tc.want)) {
				t.Fatalf("got %v total %d, want %v", got, result.Total, tc.want)
			}
		})
	}
	for _, options := range []collectorapi.CatalogOptions{
		{Categories: []string{"unknown"}}, {DefaultCategories: []string{"unknown"}},
	} {
		options.Page, options.PageSize = 1, 24
		if _, err := service.List(t.Context(), options); !errors.Is(err, panda.ErrInvalidCategory) {
			t.Fatalf("invalid category: %v", err)
		}
	}
}

func TestCatalogFactsUseLatestCurrentFavoriteEvenWithoutMetadata(t *testing.T) {
	db, service := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Category: "Manga"}, true)
	retain(t, db, panda.Metadata{ID: 3, Category: "Artist CG"}, true)
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (2, 'token2');
		INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES
		(1, 'https://panda.test', '42', 1, 'One', 0), (2, 'https://panda.test', '42', 2, 'Two', 0);
		INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES
		(1, 1, 'token1', 100), (2, 1, 'token1', 300), (1, 2, 'token2', 200)`); err != nil {
		t.Fatal(err)
	}
	newest, metadataMissing := int64(300), int64(200)
	want := []collectorapi.CatalogFact{
		{GalleryID: 1, Category: "manga", FavoritedAt: &newest},
		{GalleryID: 2, FavoritedAt: &metadataMissing},
		{GalleryID: 3, Category: "artist cg"},
		{GalleryID: 4},
	}
	got, err := service.Facts(t.Context(), []int64{1, 2, 3, 4, 1})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("facts = %+v, %v; want %+v", got, err, want)
	}
	if _, err := db.Exec("DELETE FROM favorites WHERE gallery_id = 1"); err != nil {
		t.Fatal(err)
	}
	got, err = service.Facts(t.Context(), []int64{1})
	if err != nil || len(got) != 1 || got[0].FavoritedAt != nil || got[0].Category != "manga" {
		t.Fatalf("removed favorite = %+v, %v", got, err)
	}
	for _, ids := range [][]int64{{0}, {-1}, make([]int64, collectorapi.MaxCatalogFactsSize+1)} {
		if _, err := service.Facts(t.Context(), ids); !errors.Is(err, ErrInvalidFactsBatch) {
			t.Fatalf("invalid facts batch: %v", err)
		}
	}
}
