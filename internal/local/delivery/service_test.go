package delivery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

type fixture struct {
	t         *testing.T
	db        *sql.DB
	root      string
	lib       library.Library
	collector *fakeCollector
	imports   *fakeImporter
	service   *Service
	events    []string
	mu        sync.Mutex
}

type fakeCollector struct {
	f             *fixture
	openError     map[int64]error
	deleteError   map[int64]error
	contentLength map[int64]int64
	beforeOpen    func(int64)
}

func (c *fakeCollector) OpenDownload(ctx context.Context, id int64, _ string, _ http.Header) (*http.Response, error) {
	c.f.event(fmt.Sprintf("transfer:%d", id))
	if c.beforeOpen != nil {
		c.beforeOpen(id)
	}
	if err := c.openError[id]; err != nil {
		return nil, err
	}
	data := fmt.Sprintf("archive %d", id)
	length := int64(len(data))
	if override, ok := c.contentLength[id]; ok {
		length = override
	}
	return &http.Response{StatusCode: 200, ContentLength: length, Body: io.NopCloser(strings.NewReader(data))}, nil
}

func (c *fakeCollector) DeleteDownload(_ context.Context, id int64) error {
	c.f.event(fmt.Sprintf("delete:%d", id))
	return c.deleteError[id]
}

type fakeImporter struct {
	f      *fixture
	errors map[int64]error
}

func (i *fakeImporter) ImportArchive(ctx context.Context, libraryID int64, path string) error {
	var id int64
	if _, err := fmt.Sscanf(path, "[%d].zip", &id); err != nil {
		return err
	}
	i.f.event(fmt.Sprintf("import:%d", id))
	if err := i.errors[id]; err != nil {
		return err
	}
	_, err := i.f.db.ExecContext(ctx, "INSERT INTO sources (library_id, path, kind) VALUES (?, ?, 'archive') ON CONFLICT DO NOTHING", libraryID, path)
	return err
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &fixture{t: t, db: db, root: filepath.Join(t.TempDir(), "library")}
	if err := os.Mkdir(f.root, 0o700); err != nil {
		t.Fatal(err)
	}
	f.lib, err = library.NewSQLiteRepository(db).Create(t.Context(), "Destination", f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.collector = &fakeCollector{f: f, openError: map[int64]error{}, deleteError: map[int64]error{}}
	f.imports = &fakeImporter{f: f, errors: map[int64]error{}}
	f.service = f.newService()
	return f
}

// Most worker tests step synchronously so crash and failure boundaries do not
// depend on timers. The public constructor is covered by the stop/restart test.
func (f *fixture) newService() *Service {
	ctx, cancel := context.WithCancel(f.t.Context())
	s := &Service{db: f.db, collector: f.collector, libraries: library.NewSQLiteRepository(f.db), importer: f.imports,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	f.t.Cleanup(s.Close)
	return s
}

func (f *fixture) event(value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, value)
}

func (f *fixture) start(ids ...int64) Batch {
	f.t.Helper()
	b, err := f.service.Start(f.t.Context(), f.lib.ID, ids)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

func (f *fixture) drain(id int64) Batch {
	f.t.Helper()
	for range 30 {
		more, err := f.service.step()
		if err != nil {
			f.t.Fatal(err)
		}
		if !more {
			b, err := f.service.Get(f.t.Context(), id)
			if err != nil && !errors.Is(err, ErrNotFound) {
				f.t.Fatal(err)
			}
			return b
		}
	}
	f.t.Fatal("worker did not settle")
	return Batch{}
}

func (f *fixture) get(id int64) Batch {
	f.t.Helper()
	b, err := f.service.Get(f.t.Context(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

func (f *fixture) assertRemoved(id int64) {
	f.t.Helper()
	if _, err := f.service.Get(f.t.Context(), id); !errors.Is(err, ErrNotFound) {
		f.t.Fatalf("finished delivery remains: %v", err)
	}
	var items int
	if err := f.db.QueryRow("SELECT count(*) FROM panda_delivery_items WHERE delivery_id = ?", id).Scan(&items); err != nil || items != 0 {
		f.t.Fatalf("finished delivery checkpoints: %d, %v", items, err)
	}
}

func TestDeliverySequentiallyImportsBeforeDeletingAndCleansUpBatch(t *testing.T) {
	f := newFixture(t)
	b := f.start(7, 8, 7)
	if len(b.Items) != 2 {
		t.Fatalf("batch: %+v", b)
	}
	f.drain(b.ID)
	f.assertRemoved(b.ID)
	if batches, err := f.service.List(t.Context()); err != nil || len(batches) != 0 {
		t.Fatalf("finished deliveries listed: %+v, %v", batches, err)
	}
	want := []string{"transfer:7", "import:7", "delete:7", "transfer:8", "import:8", "delete:8"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events: %v", f.events)
	}
	for _, id := range []int64{7, 8} {
		data, err := os.ReadFile(filepath.Join(f.root, archiveName(id)))
		if err != nil || string(data) != fmt.Sprintf("archive %d", id) {
			t.Fatalf("archive %d: %q %v", id, data, err)
		}
	}
}

func TestDeliverySkipsCatalogedGalleryAndPreservesCollision(t *testing.T) {
	f := newFixture(t)
	other, err := library.NewSQLiteRepository(f.db).Create(t.Context(), "Offline", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("INSERT INTO sources (library_id, path, kind) VALUES (?, 'Title [7].cbz', 'archive')", other.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(other.Path); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "[8].zip")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := f.start(7, 8, 9)
	b = f.drain(b.ID)
	if b.Items[0].State != "skipped" || b.Items[0].Error != "Already in library" || b.Items[1].State != "failed" || b.Items[2].State != "completed" {
		t.Fatalf("items: %+v", b.Items)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing" {
		t.Fatalf("collision changed: %q %v", data, err)
	}
	if !reflect.DeepEqual(f.events, []string{"transfer:9", "import:9", "delete:9"}) {
		t.Fatalf("events: %v", f.events)
	}
}

func TestStopAfterLastArchiveCleansUpFinishedBatch(t *testing.T) {
	for _, cleanupFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("cleanupFailure=%v", cleanupFailure), func(t *testing.T) {
			f := newFixture(t)
			if cleanupFailure {
				f.collector.deleteError[7] = errors.New("collector unavailable")
			}
			b := f.start(7)
			if _, err := f.service.step(); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Stop(t.Context(), b.ID); err != nil {
				t.Fatal(err)
			}
			stopped := f.drain(b.ID)
			if cleanupFailure {
				if stopped.State != "stopped" || stopped.Items[0].State != "cleanup_pending" {
					t.Fatalf("cleanup lost after stop: %+v", stopped)
				}
				delete(f.collector.deleteError, 7)
				if _, err := f.service.RetryCleanup(t.Context(), b.ID); err != nil {
					t.Fatal(err)
				}
				f.drain(b.ID)
			}
			f.assertRemoved(b.ID)
		})
	}
}

func TestRetryImportReusesSavedArchiveAcrossRestart(t *testing.T) {
	f := newFixture(t)
	f.imports.errors[7] = errors.New("import failed")
	b := f.start(7, 8)
	b = f.drain(b.ID)
	if b.State != "completed_with_errors" || b.Items[0].State != "failed" || b.Items[0].checkpoint.Stage != "saved" || b.Items[1].State != "completed" {
		t.Fatalf("batch: %+v", b)
	}
	f.service.Close()
	f.service = f.newService()
	delete(f.imports.errors, 7)
	if _, err := f.service.Retry(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	f.drain(b.ID)
	f.assertRemoved(b.ID)
	want := []string{"transfer:7", "import:7", "transfer:8", "import:8", "delete:8", "import:7", "delete:7"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events: %v", f.events)
	}
}

func TestFailedTransfersLeaveNoFinalFileAndContinue(t *testing.T) {
	f := newFixture(t)
	f.collector.openError[7] = errors.New("collector disconnected")
	f.collector.contentLength = map[int64]int64{8: 100}
	b := f.start(7, 8, 9)
	b = f.drain(b.ID)
	if b.Items[0].State != "failed" || b.Items[1].State != "failed" || b.Items[2].State != "completed" {
		t.Fatalf("items: %+v", b.Items)
	}
	for _, item := range b.Items[:2] {
		for _, path := range []string{filepath.Join(f.root, archiveName(item.GalleryID)), stagingPath(b, item)} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed transfer left %s: %v", path, err)
			}
		}
	}
	want := []string{"transfer:7", "transfer:8", "transfer:9", "import:9", "delete:9"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events: %v", f.events)
	}
}

func TestCleanupFailureContinuesAndRetriesOnlyCleanup(t *testing.T) {
	f := newFixture(t)
	f.collector.deleteError[7] = errors.New("collector unavailable")
	b := f.start(7, 8)
	b = f.drain(b.ID)
	if b.State != "completed_with_errors" || b.Items[0].State != "cleanup_pending" || b.Items[1].State != "completed" {
		t.Fatalf("batch: %+v", b)
	}
	for range maxCleanupAttempts - 1 {
		_, err := f.service.change(t.Context(), b.ID, func(b *Batch) error {
			b.Items[0].checkpoint.NextCleanup = time.Time{}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		b = f.drain(b.ID)
	}
	if b.Items[0].CleanupAttempts != maxCleanupAttempts {
		t.Fatalf("attempts: %d", b.Items[0].CleanupAttempts)
	}
	f.service.Close()
	f.service = f.newService()
	before := len(f.events)
	f.drain(b.ID)
	if len(f.events) != before {
		t.Fatal("exhausted cleanup retried after restart")
	}
	// A lost successful deletion response may produce a 404 on explicit retry.
	f.collector.deleteError[7] = &collectorapi.HTTPError{StatusCode: 404}
	if _, err := f.service.RetryCleanup(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	f.drain(b.ID)
	f.assertRemoved(b.ID)
	if f.events[len(f.events)-1] != "delete:7" || len(f.events) != before+1 {
		t.Fatalf("cleanup retry: %+v events=%v", b, f.events)
	}
}

func TestCleanupRetainsCollectorCopyWhenLocalArchiveChanged(t *testing.T) {
	f := newFixture(t)
	f.collector.deleteError[7] = errors.New("collector unavailable")
	b := f.start(7)
	f.drain(b.ID)
	if err := os.WriteFile(filepath.Join(f.root, "[7].zip"), []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	delete(f.collector.deleteError, 7)
	if _, err := f.service.RetryCleanup(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	before := len(f.events)
	b = f.drain(b.ID)
	if b.Items[0].State != "cleanup_pending" || !strings.Contains(b.Items[0].Error, "changed") || len(f.events) != before {
		t.Fatalf("unsafe cleanup: %+v events=%v", b.Items[0], f.events)
	}
}

func TestUnavailableLibraryRequiresExplicitResumeAcrossRestart(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(f.root); err != nil {
		t.Fatal(err)
	}
	b := f.start(7)
	if b.State != "paused" {
		t.Fatalf("batch: %+v", b)
	}
	if _, err := f.service.Start(t.Context(), f.lib.ID, []int64{8}); !errors.Is(err, ErrActive) {
		t.Fatalf("second active batch: %v", err)
	}
	f.service.Close()
	f.service = f.newService()
	if err := os.Mkdir(f.root, 0o700); err != nil {
		t.Fatal(err)
	}
	b = f.drain(b.ID)
	if b.State != "paused" || len(f.events) != 0 {
		t.Fatalf("implicitly resumed: %+v events=%v", b, f.events)
	}
	if _, err := f.service.Resume(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	f.drain(b.ID)
	f.assertRemoved(b.ID)
}

func TestInterruptedFinalizationRecognizesOwnedFile(t *testing.T) {
	for _, finalized := range []bool{false, true} {
		t.Run(fmt.Sprintf("finalized=%v", finalized), func(t *testing.T) {
			f := newFixture(t)
			b := f.start(7)
			item := b.Items[0]
			if err := f.service.download(b, &item); err != nil {
				t.Fatal(err)
			}
			item.State, item.checkpoint.Stage = "transferred", "transferred"
			if err := f.service.updateItem(b, item); err != nil {
				t.Fatal(err)
			}
			if finalized {
				if err := finalize(b, item); err != nil {
					t.Fatal(err)
				}
			}
			f.service.Close()
			f.service = f.newService()
			f.drain(b.ID)
			f.assertRemoved(b.ID)
			if !reflect.DeepEqual(f.events, []string{"transfer:7", "import:7", "delete:7"}) {
				t.Fatalf("events: %v", f.events)
			}
		})
	}
}

func TestRestartCompletesImportedArchiveBeforeHonoringStop(t *testing.T) {
	f := newFixture(t)
	b := f.start(7, 8)
	item := b.Items[0]
	if err := f.service.download(b, &item); err != nil {
		t.Fatal(err)
	}
	if err := finalize(b, item); err != nil {
		t.Fatal(err)
	}
	if err := f.imports.ImportArchive(t.Context(), b.LibraryID, "[7].zip"); err != nil {
		t.Fatal(err)
	}
	// Import committed, but its completion checkpoint was interrupted. The
	// durable current item must finish even if Stop was requested before restart.
	item.State, item.checkpoint.Stage = "importing", "saved"
	_, err := f.service.change(t.Context(), b.ID, func(current *Batch) error {
		current.Items[0] = item
		current.StopRequested, current.CurrentGalleryID = true, 7
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.service.Close()
	f.service = f.newService()
	b = f.drain(b.ID)
	if b.State != "stopped" || b.Items[0].State != "completed" || b.Items[1].State != "queued" {
		t.Fatalf("batch: %+v", b)
	}
	var sources int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM sources WHERE library_id = ?", b.LibraryID).Scan(&sources); err != nil || sources != 1 {
		t.Fatalf("import duplicated: %d %v", sources, err)
	}
	want := []string{"transfer:7", "import:7", "import:7", "delete:7"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events: %v", f.events)
	}
}

func TestStopFinishesCurrentArchiveAndRestartResumesQueuedWork(t *testing.T) {
	f := newFixture(t)
	f.service.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	f.collector.beforeOpen = func(id int64) {
		if id == 7 {
			close(entered)
			<-release
		}
	}
	s, err := New(t.Context(), f.db, f.collector, library.NewSQLiteRepository(f.db), f.imports, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	f.service = s
	t.Cleanup(s.Close)
	b := f.start(7, 8)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("transfer did not start")
	}
	if _, err := s.Stop(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		b = f.get(b.ID)
		if b.State == "stopped" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stop did not settle: %+v", b)
		}
		time.Sleep(time.Millisecond)
	}
	s.Close()
	if b.Items[0].State != "completed" || b.Items[1].State != "queued" {
		t.Fatalf("stop: %+v", b.Items)
	}
	f.service = f.newService()
	if _, err := f.service.Retry(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	f.drain(b.ID)
	f.assertRemoved(b.ID)
}
