package scan

import (
	"os"
	"sync"
	"testing"
)

func TestImportArchiveRetriesAndConcurrentImport(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeArchive(t, l.Path, "[42].zip", "1.jpg", "notes.txt")
	s := f.scanner(t, os.DirFS)
	if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER fail_delivery_page BEFORE INSERT ON gallery_pages BEGIN SELECT RAISE(ABORT, 'test page failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportArchive(t.Context(), l.ID, "[42].zip"); err == nil {
		t.Fatal("expected import failure")
	}
	registered, err := f.sources.List(t.Context(), l.ID)
	if err != nil || len(registered) != 0 {
		t.Fatalf("partial import: %+v, %v", registered, err)
	}
	if _, err := f.db.ExecContext(t.Context(), "DROP TRIGGER fail_delivery_page"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			if err := s.ImportArchive(t.Context(), l.ID, "[42].zip"); err != nil {
				t.Errorf("concurrent import: %v", err)
			}
		})
	}
	workers.Wait()
	registered, err = f.sources.List(t.Context(), l.ID)
	if err != nil || len(registered) != 1 {
		t.Fatalf("sources: %+v, %v", registered, err)
	}
	galleries, err := f.galleries.List(t.Context())
	if err != nil || len(galleries) != 1 {
		t.Fatalf("galleries: %+v, %v", galleries, err)
	}
	files, err := f.sources.Files(t.Context(), registered[0].ID)
	if err != nil || len(files) != 2 {
		t.Fatalf("inventory: %+v, %v", files, err)
	}
}

func TestScanExcludesDeliveryStaging(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeFile(t, l.Path, ".tana-delivery/unfinished.part")
	writeArchive(t, l.Path, ".tana-delivery/unfinished.zip", "1.jpg")
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Discovered != 0 {
		t.Fatalf("staging cataloged: %+v", status)
	}
	writeFile(t, l.Path, "loose.jpg")
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Imported != 1 || status.Discovered != 1 {
		t.Fatalf("staging hides loose images: %+v", status)
	}
	writeArchive(t, l.Path, "[42].zip", "1.jpg")
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Imported != 1 || status.Discovered != 1 {
		t.Fatalf("published archive missing: %+v", status)
	}
}
