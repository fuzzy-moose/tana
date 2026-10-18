package gallery

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/source"
)

func TestBrowseSearchPaginationAndEmptyGalleries(t *testing.T) {
	_, _, _, galleries := openRepositories(t)
	for _, title := range []string{"zebra", "Alpha", "alpha two", "100%"} {
		if _, err := galleries.Create(t.Context(), title, nil); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := galleries.Browse(t.Context(), "", 1, 2)
	if err != nil || listing.Total != 4 || len(listing.Items) != 2 || listing.Items[0].Title != "100%" || listing.Items[1].Title != "Alpha" || listing.Items[0].PageCount != 0 {
		t.Fatalf("first page: %+v, %v", listing, err)
	}
	listing, err = galleries.Browse(t.Context(), "ALPHA", 1, 24)
	if err != nil || listing.Total != 2 || listing.Items[0].Title != "Alpha" || listing.Items[1].Title != "alpha two" {
		t.Fatalf("search: %+v, %v", listing, err)
	}
	listing, err = galleries.Browse(t.Context(), "%", 1, 24)
	if err != nil || listing.Total != 1 || listing.Items[0].Title != "100%" {
		t.Fatalf("literal search: %+v, %v", listing, err)
	}
	listing, err = galleries.Browse(t.Context(), "", 999, 2)
	if err != nil || listing.Page != 2 || listing.Items[0].Title != "alpha two" || listing.Items[1].Title != "zebra" {
		t.Fatalf("clamped page: %+v, %v", listing, err)
	}
}

func TestReadingCrossLibraryOccurrencesAndRenumbering(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	var libraryIDs, fileIDs []int64
	for _, body := range []string{"first", "second"} {
		l, _, files := inventory(t, libraries, sources, ".", source.Directory, "1.png")
		if err := os.MkdirAll(l.Path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(l.Path, "1.png"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		libraryIDs = append(libraryIDs, l.ID)
		fileIDs = append(fileIDs, files["1.png"])
	}
	g, err := galleries.Create(t.Context(), "Combined", []int64{fileIDs[0], fileIDs[1], fileIDs[0]})
	if err != nil {
		t.Fatal(err)
	}
	read := func(count int64) []string {
		t.Helper()
		var result []string
		for page := int64(1); page <= count; page++ {
			content, err := galleries.OpenImage(t.Context(), g.ID, page)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(content)
			content.Close()
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, string(body))
		}
		return result
	}
	if got := read(3); !reflect.DeepEqual(got, []string{"first", "second", "first"}) {
		t.Fatalf("occurrences: %v", got)
	}
	if err := libraries.Delete(t.Context(), libraryIDs[0]); err != nil {
		t.Fatal(err)
	}
	if got := read(1); !reflect.DeepEqual(got, []string{"second"}) {
		t.Fatalf("renumbered image: %v", got)
	}
	summary, err := galleries.Summary(t.Context(), g.ID)
	if err != nil || summary.PageCount != 1 {
		t.Fatalf("renumbered count: %+v, %v", summary, err)
	}
	if err := libraries.Delete(t.Context(), libraryIDs[1]); err != nil {
		t.Fatal(err)
	}
	summary, err = galleries.Summary(t.Context(), g.ID)
	if err != nil || summary.PageCount != 0 {
		t.Fatalf("empty gallery: %+v, %v", summary, err)
	}
}
