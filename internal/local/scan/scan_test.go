package scan

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

type fixture struct {
	db        *sql.DB
	libraries *library.SQLiteRepository
	sources   *source.SQLiteRepository
	galleries *gallery.SQLiteRepository
	logger    *slog.Logger
}

func setup(t *testing.T) fixture {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return fixture{db, library.NewSQLiteRepository(db), source.NewSQLiteRepository(db), gallery.NewSQLiteRepository(db), slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func (f fixture) scanner(t *testing.T, dirFS func(string) fs.FS) *Service {
	t.Helper()
	s := New(t.Context(), f.db, f.libraries, dirFS, f.logger)
	t.Cleanup(s.Close)
	return s
}

func (f fixture) library(t *testing.T, name string) library.Library {
	t.Helper()
	l, err := f.libraries.Create(t.Context(), name, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func writeFile(t *testing.T, root, name string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("inventory only"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeArchive(t *testing.T, root, name string, entries ...string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(full)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for _, name := range entries {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Ext(name) != "" {
			if _, err := io.WriteString(entry, "inventory only; even nested ZIPs stay unopened"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func awaitFinished(t *testing.T, s *Service) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := s.Status()
		switch status.Phase {
		case "completed", "completed_with_errors", "failed":
			if status.StartedAt == nil || status.FinishedAt == nil || status.FinishedAt.Before(*status.StartedAt) {
				t.Fatalf("invalid timestamps: %+v", status)
			}
			if status.Discovered != status.Imported+status.FailedSources+status.Skipped {
				t.Fatalf("unaccounted candidates: %+v", status)
			}
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan did not finish: %+v", status)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScanInventoryAndNewPathsOnly(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	for _, name := range []string{"loose.jpg", "parent/loose.jpg", "parent/chapter/1.jpg", "mixed/loose.jpg", "leaf/10.png", "leaf/2.JPG", "leaf/notes.txt", "notes/readme.txt", ".hidden/.page.png", "bad.zip"} {
		writeFile(t, l.Path, name)
	}
	if err := os.Mkdir(filepath.Join(l.Path, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeArchive(t, l.Path, "Book.CBZ", "chapter2/", "chapter10/1.jpg", "chapter2/2.jpg", "nested/inner.zip", "notes.txt")
	writeArchive(t, l.Path, "mixed/a.zip", "cover.PNG")
	writeArchive(t, l.Path, "mixed/b.CBZ")
	writeArchive(t, l.Path, "duplicate.zip", "1.jpg", "1.jpg")
	writeArchive(t, l.Path, "unsafe.zip", "../1.jpg")
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	status := awaitFinished(t, s)
	if status.Phase != "completed_with_errors" || status.Discovered != 10 || status.Imported != 7 || status.FailedSources != 3 || status.GalleriesCreated != 5 {
		t.Fatalf("scan results: %+v", status)
	}
	want := map[string][]string{
		"Book.CBZ":       {"chapter10/1.jpg", "chapter2/2.jpg", "nested/inner.zip", "notes.txt"},
		"mixed/a.zip":    {"cover.PNG"},
		"mixed/b.CBZ":    {},
		"parent/chapter": {"1.jpg"},
		"leaf":           {"10.png", "2.JPG", "notes.txt"},
		"notes":          {"readme.txt"},
		".hidden":        {".page.png"},
	}
	registered, err := f.sources.List(t.Context(), l.ID)
	if err != nil || len(registered) != len(want) {
		t.Fatalf("sources: %+v, %v", registered, err)
	}
	filePaths := map[int64]string{}
	for _, imported := range registered {
		expected, ok := want[imported.Path]
		if !ok {
			t.Fatalf("unexpected source: %+v", imported)
		}
		files, err := f.sources.Files(t.Context(), imported.ID)
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, file := range files {
			paths = append(paths, file.Path)
			filePaths[file.ID] = file.Path
		}
		if !slices.Equal(paths, expected) {
			t.Fatalf("%s inventory: %v, want %v", imported.Path, paths, expected)
		}
	}
	galleries, err := f.galleries.List(t.Context())
	if err != nil || len(galleries) != 5 {
		t.Fatalf("galleries: %+v, %v", galleries, err)
	}
	for _, g := range galleries {
		if g.Title == "Book" {
			pages, err := f.galleries.Pages(t.Context(), g.ID)
			if err != nil || len(pages) != 2 || filePaths[pages[0].SourceFileID] != "chapter2/2.jpg" || filePaths[pages[1].SourceFileID] != "chapter10/1.jpg" {
				t.Fatalf("automatic page order: %+v, %v", pages, err)
			}
		}
	}
	// Existing inventories stay fixed; failed and newly discovered paths retry.
	writeFile(t, l.Path, "leaf/3.jpg")
	writeFile(t, l.Path, "new/readme.txt")
	writeArchive(t, l.Path, "bad.zip", "1.jpg")
	if err := s.Request(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	status = awaitFinished(t, s)
	if status.Discovered != 4 || status.Imported != 2 || status.FailedSources != 2 || status.GalleriesCreated != 1 {
		t.Fatalf("repeat scan: %+v", status)
	}
	for _, imported := range registered {
		if imported.Path == "leaf" {
			files, err := f.sources.Files(t.Context(), imported.ID)
			if err != nil || len(files) != 3 {
				t.Fatalf("existing inventory changed: %+v, %v", files, err)
			}
		}
	}
	remaining, err := f.galleries.List(t.Context())
	if err != nil || len(remaining) != 6 {
		t.Fatalf("duplicate galleries: %+v, %v", remaining, err)
	}
}

func TestScanAllContinuesUnavailableLibraryAndRootSource(t *testing.T) {
	f := setup(t)
	a, b := f.library(t, "A"), f.library(t, "B")
	writeFile(t, a.Path, "1.jpg")
	if err := os.Remove(b.Path); err != nil {
		t.Fatal(err)
	}
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	status := awaitFinished(t, s)
	if status.Phase != "completed_with_errors" || status.LibrariesTotal != 2 || status.DiscoveryErrors != 1 || status.Imported != 1 || status.GalleriesCreated != 1 {
		t.Fatalf("scan all: %+v", status)
	}
	registered, err := f.sources.List(t.Context(), a.ID)
	if err != nil || len(registered) != 1 || registered[0].Path != "." {
		t.Fatalf("root source: %+v, %v", registered, err)
	}
	if err := s.Request(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	status = awaitFinished(t, s)
	if status.Phase != "completed_with_errors" || status.LibrariesTotal != 1 || status.DiscoveryErrors != 1 || status.Discovered != 0 {
		t.Fatalf("single unavailable library: %+v", status)
	}
}

func TestGalleryFailureRollsBackWholeSourceAndCanRetry(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeFile(t, l.Path, "book/1.jpg")
	writeFile(t, l.Path, "notes/readme.txt")
	if _, err := f.db.ExecContext(t.Context(), `CREATE TRIGGER fail_page BEFORE INSERT ON gallery_pages BEGIN SELECT RAISE(ABORT, 'test page failure'); END`); err != nil {
		t.Fatal(err)
	}
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	status := awaitFinished(t, s)
	if status.Imported != 1 || status.FailedSources != 1 || status.GalleriesCreated != 0 {
		t.Fatalf("failure results: %+v", status)
	}
	registered, err := f.sources.List(t.Context(), l.ID)
	if err != nil || len(registered) != 1 || registered[0].Path != "notes" {
		t.Fatalf("partial source survived: %+v, %v", registered, err)
	}
	var files, galleries int
	if err := f.db.QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM source_files), (SELECT count(*) FROM galleries)").Scan(&files, &galleries); err != nil || files != 1 || galleries != 0 {
		t.Fatalf("partial records: files=%d galleries=%d, %v", files, galleries, err)
	}
	if _, err := f.db.ExecContext(t.Context(), "DROP TRIGGER fail_page"); err != nil {
		t.Fatal(err)
	}
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	status = awaitFinished(t, s)
	if status.Phase != "completed" || status.Discovered != 1 || status.Imported != 1 || status.GalleriesCreated != 1 {
		t.Fatalf("retry: %+v", status)
	}
}

type gatedFS struct {
	fs.FS
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (f *gatedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	f.once.Do(func() {
		close(f.entered)
		<-f.release
	})
	return fs.ReadDir(f.FS, name)
}

func awaitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reach filesystem gate")
	}
}

func TestDiscoveryFinishesBeforeImportAndSnapshotsLibraries(t *testing.T) {
	f := setup(t)
	a, b := f.library(t, "A"), f.library(t, "B")
	writeFile(t, a.Path, "1.jpg")
	writeFile(t, b.Path, "2.jpg")
	gate := &gatedFS{FS: os.DirFS(b.Path), entered: make(chan struct{}), release: make(chan struct{})}
	s := f.scanner(t, func(root string) fs.FS {
		if root == b.Path {
			return gate
		}
		return os.DirFS(root)
	})
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	requestCtx, cancel := context.WithCancel(t.Context())
	if err := s.Request(requestCtx, 0); err != nil {
		t.Fatal(err)
	}
	cancel() // Disconnecting the requester does not cancel accepted work.
	awaitSignal(t, gate.entered)
	status := s.Status()
	if status.Phase != "discovering" || status.Discovered != 1 || status.Imported != 0 {
		t.Fatalf("premature import: %+v", status)
	}
	registered, err := f.sources.List(t.Context(), a.ID)
	if err != nil || len(registered) != 0 {
		t.Fatalf("database changed during discovery: %+v, %v", registered, err)
	}
	for _, id := range []int64{0, a.ID, b.ID} {
		if err := s.Request(t.Context(), id); !errors.Is(err, ErrActive) {
			t.Fatalf("overlapping request %d: %v", id, err)
		}
	}
	c := f.library(t, "C")
	writeFile(t, c.Path, "3.jpg")
	release.Do(func() { close(gate.release) })
	status = awaitFinished(t, s)
	if status.Phase != "completed" || status.LibrariesTotal != 2 || status.Imported != 2 {
		t.Fatalf("library snapshot changed: %+v", status)
	}
	registered, err = f.sources.List(t.Context(), c.ID)
	if err != nil || len(registered) != 0 {
		t.Fatalf("new library included in active scan: %+v, %v", registered, err)
	}
	s.Close()
	restarted := f.scanner(t, os.DirFS)
	if got := restarted.Status(); !reflect.DeepEqual(got, Status{Phase: "idle"}) {
		t.Fatalf("restart retained status: %+v", got)
	}
	if err := restarted.Request(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	status = awaitFinished(t, restarted)
	if status.Imported != 1 || status.Discovered != 1 || status.LibrariesTotal != 3 {
		t.Fatalf("fresh scan after restart: %+v", status)
	}
}

type archiveGateFS struct {
	fs.FS
	opened  chan string
	release chan struct{}
}

func (f *archiveGateFS) Open(name string) (fs.File, error) {
	if source.IsArchive(name) {
		f.opened <- name
		<-f.release
	}
	return f.FS.Open(name)
}

func TestImportIOIsBoundedAndUsesDiscoveredSourceList(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	for i := range 9 {
		writeArchive(t, l.Path, fmt.Sprintf("%d.cbz", i), "1.jpg")
	}
	gate := &archiveGateFS{FS: os.DirFS(l.Path), opened: make(chan string, 10), release: make(chan struct{})}
	s := f.scanner(t, func(string) fs.FS { return gate })
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		select {
		case <-gate.opened:
		case <-time.After(5 * time.Second):
			t.Fatal("expected concurrent inventory reads")
		}
	}
	select {
	case name := <-gate.opened:
		t.Fatalf("too many concurrent inventory reads: %s", name)
	default:
	}
	if status := s.Status(); status.Phase != "importing" || status.Discovered != 9 || status.Imported != 0 {
		t.Fatalf("import status: %+v", status)
	}
	if err := s.Request(t.Context(), 0); !errors.Is(err, ErrActive) {
		t.Fatalf("second scan accepted during import: %v", err)
	}
	writeArchive(t, l.Path, "later.zip", "2.jpg")
	release.Do(func() { close(gate.release) })
	status := awaitFinished(t, s)
	if status.Phase != "completed" || status.Discovered != 9 || status.Imported != 9 || status.GalleriesCreated != 9 {
		t.Fatalf("discovered source list changed: %+v", status)
	}
}

func TestShutdownStopsScanAndLeavesUncommittedSourcesForNextScan(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeArchive(t, l.Path, "book.cbz", "1.jpg")
	gate := &archiveGateFS{FS: os.DirFS(l.Path), opened: make(chan string, 1), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := New(ctx, f.db, f.libraries, func(string) fs.FS { return gate }, f.logger)
	t.Cleanup(s.Close)
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.opened:
	case <-time.After(5 * time.Second):
		t.Fatal("import did not start")
	}
	cancel()
	release.Do(func() { close(gate.release) })
	s.Close()
	status := awaitFinished(t, s)
	if status.Phase != "failed" || status.Imported != 0 || status.Skipped != 1 {
		t.Fatalf("shutdown: %+v", status)
	}
	if err := s.Request(t.Context(), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan accepted after shutdown: %v", err)
	}
	restarted := f.scanner(t, os.DirFS)
	if err := restarted.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	status = awaitFinished(t, restarted)
	if status.Phase != "completed" || status.Imported != 1 || status.GalleriesCreated != 1 {
		t.Fatalf("scan after interruption: %+v", status)
	}
}

func TestScanSkipsSymlinksAndContinuesAfterDirectoryReadFailure(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeFile(t, l.Path, "good/1.jpg")
	writeFile(t, l.Path, "unreadable/2.jpg")
	if err := os.Symlink(filepath.Join(l.Path, "good"), filepath.Join(l.Path, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(l.Path, "unreadable", "2.jpg"), filepath.Join(l.Path, "good", "linked.jpg")); err != nil {
		t.Fatal(err)
	}
	s := f.scanner(t, func(root string) fs.FS { return unreadableFS{os.DirFS(root)} })
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	status := awaitFinished(t, s)
	if status.Phase != "completed_with_errors" || status.Imported != 1 || status.Discovered != 1 || status.DiscoveryErrors != 1 {
		t.Fatalf("read failure handling: %+v", status)
	}
	registered, err := f.sources.List(t.Context(), l.ID)
	if err != nil || len(registered) != 1 || registered[0].Path != "good" {
		t.Fatalf("sources: %+v, %v", registered, err)
	}
	files, err := f.sources.Files(t.Context(), registered[0].ID)
	if err != nil || len(files) != 1 || files[0].Path != "1.jpg" {
		t.Fatalf("symlink inventoried: %+v, %v", files, err)
	}
}

type unreadableFS struct{ fs.FS }

func (f unreadableFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "unreadable" {
		return nil, fs.ErrPermission
	}
	return fs.ReadDir(f.FS, name)
}
