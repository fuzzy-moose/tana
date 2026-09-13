package refresh

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

type fixture struct {
	db      *sql.DB
	service *Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return fixture{db: db, service: New(db, os.DirFS)}
}

func (f fixture) library(t *testing.T, name string) library.Library {
	t.Helper()
	l, err := library.NewSQLiteRepository(f.db).Create(t.Context(), name, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (f fixture) source(t *testing.T, l library.Library, name string, kind source.Kind, present bool) (source.Source, gallery.Gallery, int64) {
	t.Helper()
	if present {
		path := filepath.Join(l.Path, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		var err error
		if kind == source.Directory {
			err = os.MkdirAll(path, 0o700) // Empty directories must be retained.
		} else {
			err = os.WriteFile(path, []byte("contents need not be a valid archive"), 0o600)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	repo := source.NewSQLiteRepository(f.db)
	s, err := repo.Create(t.Context(), l.ID, name, kind, []string{"1.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := gallery.NewSQLiteRepository(f.db).CreateFromSource(t.Context(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	files, err := repo.Files(t.Context(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, g, files[0].ID
}

func (f fixture) preview(t *testing.T, libraryID int64, want int) Preview {
	t.Helper()
	p, err := f.service.Preview(t.Context(), libraryID)
	if err != nil || len(p.Candidates) != want || p.PlanID == "" {
		t.Fatalf("preview = %+v, %v; want %d candidates", p, err, want)
	}
	return p
}

func TestRefreshRemovesSelectedMissingSourcesAndTheirGalleryReferences(t *testing.T) {
	f := newFixture(t)
	l := f.library(t, "Manga")
	a, linked, aFile := f.source(t, l, "Gone.cbz", source.Archive, false)
	b, _, _ := f.source(t, l, "Removed/Directory", source.Directory, false)
	excluded, _, _ := f.source(t, l, "Excluded.zip", source.Archive, false)
	present, _, presentFile := f.source(t, l, "Present.cbz", source.Archive, true)
	empty, _, _ := f.source(t, l, "Empty", source.Directory, true)
	galleries := gallery.NewSQLiteRepository(f.db)
	combined, err := galleries.Create(t.Context(), "Combined", []int64{aFile, presentFile, aFile})
	if err != nil {
		t.Fatal(err)
	}
	independent, err := galleries.Create(t.Context(), "Independent", []int64{aFile})
	if err != nil {
		t.Fatal(err)
	}
	other := f.library(t, "Other")
	otherMissing, _, _ := f.source(t, other, "Gone.zip", source.Archive, false)
	otherPresent, _, _ := f.source(t, other, "Present.zip", source.Archive, true)

	preview := f.preview(t, l.ID, 3)
	for _, candidate := range preview.Candidates {
		if candidate.LibraryID != l.ID || candidate.LibraryName != l.Name || !filepath.IsAbs(candidate.Path) {
			t.Fatalf("wrong library or path: %+v", candidate)
		}
		if candidate.SourceID == a.ID {
			want := []Gallery{
				{ID: linked.ID, Title: linked.Title, Deleted: true, PagesRemoved: 1},
				{ID: combined.ID, Title: combined.Title, PagesRemoved: 2},
				{ID: independent.ID, Title: independent.Title, PagesRemoved: 1},
			}
			if !reflect.DeepEqual(candidate.Galleries, want) {
				t.Fatalf("affected galleries = %+v; want %+v", candidate.Galleries, want)
			}
		}
	}
	if _, err := source.NewSQLiteRepository(f.db).Get(t.Context(), a.ID); err != nil {
		t.Fatalf("preview changed catalog: %v", err)
	}
	preview = f.preview(t, 0, 4)
	result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{a.ID, b.ID, otherMissing.ID})
	if err != nil || !reflect.DeepEqual(result.Removed, []int64{a.ID, b.ID, otherMissing.ID}) || len(result.Failed) != 0 {
		t.Fatalf("execute = %+v, %v", result, err)
	}
	for _, id := range []int64{a.ID, b.ID, otherMissing.ID} {
		if _, err := source.NewSQLiteRepository(f.db).Get(t.Context(), id); !errors.Is(err, source.ErrNotFound) {
			t.Fatalf("removed source %d remains: %v", id, err)
		}
	}
	if _, err := galleries.Get(t.Context(), linked.ID); !errors.Is(err, gallery.ErrNotFound) {
		t.Fatalf("linked gallery remains: %v", err)
	}
	pages, err := galleries.Pages(t.Context(), combined.ID)
	if err != nil || len(pages) != 1 || pages[0].Number != 1 || pages[0].SourceFileID != presentFile {
		t.Fatalf("independent gallery pages = %+v, %v", pages, err)
	}
	if _, err := galleries.Get(t.Context(), independent.ID); err != nil {
		t.Fatalf("empty independent gallery lost: %v", err)
	}
	if pages, err := galleries.Pages(t.Context(), independent.ID); err != nil || len(pages) != 0 {
		t.Fatalf("empty independent gallery pages = %+v, %v", pages, err)
	}
	for _, id := range []int64{excluded.ID, present.ID, empty.ID, otherPresent.ID} {
		if _, err := source.NewSQLiteRepository(f.db).Get(t.Context(), id); err != nil {
			t.Fatalf("retained source %d missing: %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(l.Path, present.Path)); err != nil {
		t.Fatalf("refresh affected existing file: %v", err)
	}
}

func TestRefreshBlocksUnavailableAndEntirelyMissingLibraries(t *testing.T) {
	f := newFixture(t)
	l := f.library(t, "Missing")
	f.source(t, l, "Missing.cbz", source.Archive, false)
	preview := f.preview(t, 0, 0)
	if len(preview.Skipped) != 1 || preview.Skipped[0].LibraryID != l.ID || preview.Skipped[0].SourceID != 0 {
		t.Fatalf("empty library not blocked: %+v", preview)
	}
	if err := os.Remove(l.Path); err != nil {
		t.Fatal(err)
	}
	preview = f.preview(t, l.ID, 0)
	if len(preview.Skipped) != 1 {
		t.Fatalf("unavailable root not blocked: %+v", preview)
	}
	if _, err := f.service.Preview(t.Context(), l.ID+1); !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("unknown library: %v", err)
	}
}

func TestRefreshRechecksStorageAndGalleryImpactAtConfirmation(t *testing.T) {
	for _, change := range []string{"restored", "disconnected", "all missing", "gallery edited", "source recataloged"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			l := f.library(t, "Manga")
			missing, linked, _ := f.source(t, l, "Gone.cbz", source.Archive, false)
			present, _, _ := f.source(t, l, "Present.zip", source.Archive, true)
			preview := f.preview(t, l.ID, 1)
			var err error
			switch change {
			case "restored":
				err = os.WriteFile(filepath.Join(l.Path, missing.Path), []byte("restored"), 0o600)
			case "disconnected":
				err = os.Rename(l.Path, filepath.Join(t.TempDir(), "offline"))
			case "all missing":
				err = os.Remove(filepath.Join(l.Path, present.Path))
			case "gallery edited":
				_, err = gallery.NewSQLiteRepository(f.db).Rename(t.Context(), linked.ID, "Edited title")
			case "source recataloged":
				err = source.NewSQLiteRepository(f.db).Delete(t.Context(), missing.ID)
				if err == nil {
					f.source(t, l, missing.Path, source.Archive, false)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{missing.ID})
			if err != nil || len(result.Removed) != 0 || len(result.Failed) != 1 {
				t.Fatalf("unsafe removal: %+v, %v", result, err)
			}
			rows, err := source.NewSQLiteRepository(f.db).List(t.Context(), l.ID)
			if err != nil || len(rows) != 2 {
				t.Fatalf("catalog changed: %+v, %v", rows, err)
			}
		})
	}
}

func TestRefreshRequiresAValidSingleUseSelection(t *testing.T) {
	f := newFixture(t)
	l := f.library(t, "Manga")
	missing, _, _ := f.source(t, l, "Gone.zip", source.Archive, false)
	present, _, _ := f.source(t, l, "Present.zip", source.Archive, true)
	preview := f.preview(t, l.ID, 1)
	for _, ids := range [][]int64{nil, {present.ID}, {missing.ID, missing.ID}, {missing.ID, 99999}} {
		if _, err := f.service.Execute(t.Context(), preview.PlanID, ids); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("accepted invalid selection %v: %v", ids, err)
		}
	}
	if _, err := f.service.Execute(t.Context(), "unknown", []int64{missing.ID}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("accepted unknown preview: %v", err)
	}
	expired := f.preview(t, l.ID, 1)
	p := f.service.plans[expired.PlanID]
	p.created = time.Now().Add(-time.Hour)
	f.service.plans[expired.PlanID] = p
	if _, err := f.service.Execute(t.Context(), expired.PlanID, []int64{missing.ID}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("accepted expired preview: %v", err)
	}
	if _, err := f.service.Execute(t.Context(), preview.PlanID, []int64{missing.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Execute(t.Context(), preview.PlanID, []int64{missing.ID}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("accepted replay: %v", err)
	}
}
