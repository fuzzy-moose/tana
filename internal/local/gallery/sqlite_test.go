package gallery

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func openRepositories(t *testing.T) (*sql.DB, *library.SQLiteRepository, *source.SQLiteRepository, *SQLiteRepository) {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, library.NewSQLiteRepository(db), source.NewSQLiteRepository(db), NewSQLiteRepository(db)
}

func inventory(t *testing.T, libraries *library.SQLiteRepository, sources *source.SQLiteRepository, sourcePath string, kind source.Kind, paths ...string) (library.Library, source.Source, map[string]int64) {
	t.Helper()
	l, err := libraries.Create(t.Context(), "Library", filepath.Join(t.TempDir(), "unavailable"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := sources.Create(t.Context(), l.ID, sourcePath, kind, paths)
	if err != nil {
		t.Fatal(err)
	}
	files, err := sources.Files(t.Context(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]int64, len(files))
	for _, file := range files {
		ids[file.Path] = file.ID
	}
	return l, s, ids
}

func assertPages(t *testing.T, r *SQLiteRepository, galleryID int64, want ...int64) {
	t.Helper()
	pages, err := r.Pages(t.Context(), galleryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != len(want) {
		t.Fatalf("pages: %+v; want file IDs %v", pages, want)
	}
	for i, page := range pages {
		if page.GalleryID != galleryID || page.Number != int64(i+1) || page.SourceFileID != want[i] {
			t.Fatalf("page %d: %+v; want %d", i+1, page, want[i])
		}
	}
}

func TestDefaultGallerySelectsSupportedImagesInNaturalOrder(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	_, s, ids := inventory(t, libraries, sources, "Series/Volume.CBZ", source.Archive,
		"chapter10/1.jpg", "chapter2/10.PNG", "chapter2/2.JPG", "chapter2/1.bmp", "chapter2/3.gif", "chapter2/4.jpeg", "chapter2/5.webp", "notes.txt", "inner.zip", "cover.avif")
	g, err := galleries.CreateFromSource(t.Context(), s.ID)
	if err != nil || g.Title != "Volume" {
		t.Fatalf("gallery: %+v, %v", g, err)
	}
	assertPages(t, galleries, g.ID, ids["chapter2/1.bmp"], ids["chapter2/2.JPG"], ids["chapter2/3.gif"], ids["chapter2/4.jpeg"], ids["chapter2/5.webp"], ids["chapter2/10.PNG"], ids["chapter10/1.jpg"])
	files, err := sources.Files(t.Context(), s.ID)
	if err != nil || len(files) != 10 {
		t.Fatalf("inventory changed: %+v, %v", files, err)
	}

	_, textSource, _ := inventory(t, libraries, sources, "Notes", source.Directory, "notes.txt")
	if _, err := galleries.CreateFromSource(t.Context(), textSource.ID); !errors.Is(err, ErrNoImages) {
		t.Fatalf("want no images: %v", err)
	}
	listed, err := galleries.List(t.Context())
	if err != nil || len(listed) != 1 {
		t.Fatalf("empty default gallery created: %+v, %v", listed, err)
	}
	if _, err := galleries.CreateFromSource(t.Context(), 999); !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("missing source: %v", err)
	}
}

func TestCrossLibraryPagesAndDeletion(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	ctx := t.Context()
	a, sa, af := inventory(t, libraries, sources, "A.zip", source.Archive, "1.jpg", "2.jpg", "unused.jpg")
	b, sb, bf := inventory(t, libraries, sources, "B", source.Directory, "1.png", "2.png")
	g, err := galleries.Create(ctx, "Combined", []int64{af["1.jpg"], bf["2.png"], af["1.jpg"], af["2.jpg"], bf["1.png"], af["2.jpg"], bf["2.png"]})
	if err != nil {
		t.Fatal(err)
	}
	other, err := galleries.Create(ctx, "Subset", []int64{af["2.jpg"], af["1.jpg"]})
	if err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID, af["1.jpg"], bf["2.png"], af["1.jpg"], af["2.jpg"], bf["1.png"], af["2.jpg"], bf["2.png"])
	assertPages(t, galleries, other.ID, af["2.jpg"], af["1.jpg"])
	if err := libraries.UpdateAvailability(ctx, a.ID, "unavailable", time.Now()); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, other.ID, af["2.jpg"], af["1.jpg"])
	if err := libraries.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID, bf["2.png"], bf["1.png"], bf["2.png"])
	assertPages(t, galleries, other.ID)
	if retained, err := galleries.Get(ctx, other.ID); err != nil || retained != other {
		t.Fatalf("empty gallery lost: %+v, %v", retained, err)
	}
	if _, err := sources.Get(ctx, sa.ID); !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("deleted library retained source: %v", err)
	}
	if files, err := sources.Files(ctx, sa.ID); err != nil || len(files) != 0 {
		t.Fatalf("deleted inventory: %+v, %v", files, err)
	}
	if _, err := sources.Get(ctx, sb.ID); err != nil {
		t.Fatal(err)
	}
	if err := libraries.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID)
	if retained, err := galleries.Get(ctx, g.ID); err != nil || retained != g {
		t.Fatalf("gallery lost: %+v, %v", retained, err)
	}
}

func TestPageReplacementIsAtomicAndGalleryDeletionPreservesSource(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	ctx := t.Context()
	_, s, ids := inventory(t, libraries, sources, "book.zip", source.Archive, "1.jpg", "2.png", "notes.txt")
	g, err := galleries.Create(ctx, "book", []int64{ids["1.jpg"], ids["2.png"]})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []int64{ids["notes.txt"], 999} {
		if err := galleries.ReplacePages(ctx, g.ID, []int64{ids["2.png"], invalid}); !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("invalid replacement: %v", err)
		}
		assertPages(t, galleries, g.ID, ids["1.jpg"], ids["2.png"])
		if _, err := galleries.Create(ctx, "Invalid", []int64{ids["1.jpg"], invalid}); !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("invalid create: %v", err)
		}
	}
	listed, err := galleries.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("failed create left a gallery: %+v, %v", listed, err)
	}
	if err := galleries.ReplacePages(ctx, 999, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing gallery: %v", err)
	}
	if err := galleries.ReplacePages(ctx, g.ID, []int64{ids["2.png"], ids["1.jpg"], ids["2.png"]}); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID, ids["2.png"], ids["1.jpg"], ids["2.png"])
	if _, err := galleries.Rename(ctx, g.ID, " "); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("blank title: %v", err)
	}
	renamed, err := galleries.Rename(ctx, g.ID, "  Edited  ")
	if err != nil || renamed.Title != "Edited" || renamed.ID != g.ID {
		t.Fatalf("rename: %+v, %v", renamed, err)
	}
	if err := galleries.ReplacePages(ctx, g.ID, nil); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID)
	if err := galleries.Delete(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := galleries.Delete(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := galleries.Get(ctx, g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted gallery: %v", err)
	}
	files, err := sources.Files(ctx, s.ID)
	if err != nil || len(files) != 3 {
		t.Fatalf("gallery deletion lost inventory: %+v, %v", files, err)
	}
	remaining, err := galleries.CreateFromSource(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sources.Delete(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, remaining.ID)
	if _, err := galleries.Get(ctx, remaining.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source deletion retained linked gallery: %v", err)
	}
}

func TestSourceLinkedGalleryMembershipAndLifecycle(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	ctx := t.Context()
	l, s, ids := inventory(t, libraries, sources, "book.zip", source.Archive, "1.jpg", "2.png", "notes.txt")
	_, _, other := inventory(t, libraries, sources, "other", source.Directory, "1.jpg")
	g, err := galleries.CreateFromSource(ctx, s.ID)
	if err != nil || g.SourceID != s.ID {
		t.Fatalf("source link: %+v, %v", g, err)
	}
	for _, pages := range [][]int64{
		nil,
		{ids["1.jpg"]},
		{ids["1.jpg"], ids["2.png"], ids["1.jpg"]},
		{ids["1.jpg"], ids["1.jpg"]},
		{ids["1.jpg"], other["1.jpg"]},
		{ids["1.jpg"], ids["notes.txt"]},
	} {
		if err := galleries.ReplacePages(ctx, g.ID, pages); !errors.Is(err, ErrLinkedPages) {
			t.Fatalf("membership change accepted: %v, %v", pages, err)
		}
		assertPages(t, galleries, g.ID, ids["1.jpg"], ids["2.png"])
	}
	if err := galleries.ReplacePages(ctx, g.ID, []int64{ids["2.png"], ids["1.jpg"]}); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID, ids["2.png"], ids["1.jpg"])
	if renamed, err := galleries.Rename(ctx, g.ID, "Edited"); err != nil || renamed.SourceID != s.ID {
		t.Fatalf("rename lost source link: %+v, %v", renamed, err)
	}
	if err := libraries.UpdateAvailability(ctx, l.ID, "unavailable", time.Now()); err != nil {
		t.Fatal(err)
	}
	assertPages(t, galleries, g.ID, ids["2.png"], ids["1.jpg"])
	if err := libraries.Delete(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := galleries.Get(ctx, g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("library deletion retained linked gallery: %v", err)
	}
}

func TestGalleryPersistence(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, _, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	libraries, sources, galleries := library.NewSQLiteRepository(db), source.NewSQLiteRepository(db), NewSQLiteRepository(db)
	_, s, _ := inventory(t, libraries, sources, "Volume.1", source.Directory, "2.jpg", "10.jpg")
	g, err := galleries.CreateFromSource(ctx, s.ID)
	if err != nil || g.Title != "Volume.1" {
		t.Fatalf("gallery: %+v, %v", g, err)
	}
	pages, err := galleries.Pages(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	galleries = NewSQLiteRepository(db)
	got, err := galleries.Get(ctx, g.ID)
	if err != nil || got != g {
		t.Fatalf("reopened gallery: %+v, %v", got, err)
	}
	gotPages, err := galleries.Pages(ctx, g.ID)
	if err != nil || !reflect.DeepEqual(gotPages, pages) {
		t.Fatalf("reopened pages: %+v, %v", gotPages, err)
	}
}

func TestReimportDoesNotReuseDeletedIDs(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	var previousLibrary, previousSource, previousFile, previousGallery int64
	for range 2 {
		l, s, files := inventory(t, libraries, sources, "book", source.Directory, "1.jpg")
		g, err := galleries.CreateFromSource(t.Context(), s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if l.ID <= previousLibrary || s.ID <= previousSource || files["1.jpg"] <= previousFile || g.ID <= previousGallery {
			t.Fatalf("IDs did not advance: library=%d source=%d file=%d gallery=%d", l.ID, s.ID, files["1.jpg"], g.ID)
		}
		previousLibrary, previousSource, previousFile, previousGallery = l.ID, s.ID, files["1.jpg"], g.ID
		if err := libraries.Delete(t.Context(), l.ID); err != nil {
			t.Fatal(err)
		}
	}
}
