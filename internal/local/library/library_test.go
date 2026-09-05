package library

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func testService(t *testing.T) *Service {
	t.Helper()
	return testServiceAt(t, t.TempDir())
}

func testServiceAt(t *testing.T, dir string) *Service {
	t.Helper()
	db, dir, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := newService(t.Context(), NewSQLiteRepository(db), os.DirFS, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestServiceLeavesDatabaseOwnershipWithCaller(t *testing.T) {
	for _, reject := range []bool{false, true} {
		name := "close"
		if reject {
			name = "initialization failure"
		}
		t.Run(name, func(t *testing.T) {
			db, dir, err := storage.Open(t.Context(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if reject {
				// A previously registered root cannot contain application storage.
				_, err := db.Exec("INSERT INTO libraries (id, name, path) VALUES (?, ?, ?)", "overlap", "Overlap", dir)
				if err != nil {
					t.Fatal(err)
				}
			}
			s, err := New(t.Context(), NewSQLiteRepository(db), os.DirFS, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if s != nil {
				s.Close()
			}
			if reject && !errors.Is(err, ErrStorageOverlap) {
				t.Fatalf("expected storage overlap, got %v", err)
			}
			if !reject && err != nil {
				t.Fatal(err)
			}
			if err := db.PingContext(t.Context()); err != nil {
				t.Fatalf("library closed the application database: %v", err)
			}
		})
	}
}

func TestRegistrationRoots(t *testing.T) {
	s := testService(t)
	parent := t.TempDir()
	root := filepath.Join(parent, "comics")
	child := filepath.Join(root, "manga")
	sibling := filepath.Join(parent, "comics-other")
	for _, path := range []string{child, sibling} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	registered, err := s.Create(t.Context(), "  Manga  ", root+string(filepath.Separator)+".")
	if err != nil {
		t.Fatal(err)
	}
	if registered.Path != root || registered.Name != "Manga" || registered.Availability != "available" || registered.LastCheckedAt == nil {
		t.Fatalf("unexpected registration: %+v", registered)
	}
	for _, path := range []string{root, child, parent, root + string(filepath.Separator)} {
		if _, err := s.Create(t.Context(), "Duplicate", path); !errors.Is(err, ErrRootConflict) {
			t.Errorf("path %q: want conflict, got %v", path, err)
		}
	}
	// Symlinks follow normal filesystem behavior; their paths stay distinct.
	linked, err := s.Create(t.Context(), "Alias", alias)
	if err != nil || linked.Path != alias {
		t.Fatalf("symlink path was rewritten or rejected: %+v %v", linked, err)
	}
	if _, err := s.Create(t.Context(), "Manga", sibling); err != nil {
		t.Fatalf("sibling root and duplicate name must be allowed: %v", err)
	}
	if _, err := s.Create(t.Context(), "Data", s.dataDir); !errors.Is(err, ErrStorageOverlap) {
		t.Fatalf("application storage accepted: %v", err)
	}
	if _, err := s.Create(t.Context(), "Data parent", filepath.Dir(s.dataDir)); !errors.Is(err, ErrStorageOverlap) {
		t.Fatalf("parent of application storage accepted: %v", err)
	}
}

func TestRegistrationOnlyRequiresExistingDirectory(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	file := filepath.Join(root, "comic.cbz")
	if err := os.WriteFile(file, []byte("comic"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		want       error
	}{
		{" ", root, ErrInvalidName},
		{"Relative", "comics", ErrInvalidPath},
		{"Missing", filepath.Join(root, "missing"), ErrRootUnavailable},
		{"File", file, ErrRootUnavailable},
	} {
		if _, err := s.Create(t.Context(), tc.name, tc.path); !errors.Is(err, tc.want) {
			t.Errorf("%q: want %v, got %v", tc.name, tc.want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatalf("registration created a directory: %v", err)
	}
	empty := t.TempDir()
	if err := os.Chmod(empty, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(empty, 0o700) })
	if _, err := s.Create(t.Context(), "No listing permission", empty); err != nil {
		t.Fatalf("existence-only registration rejected directory: %v", err)
	}
}

func TestAvailabilityOutageAndRecovery(t *testing.T) {
	s := testService(t)
	parent := t.TempDir()
	root := filepath.Join(parent, "nas")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	library, err := s.Create(t.Context(), "NAS", root)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(parent, "disconnected")
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	s.check(t.Context(), library.ID)
	unavailable, err := s.Get(t.Context(), library.ID)
	if err != nil || unavailable.Availability != "unavailable" || unavailable.LastCheckedAt == nil {
		t.Fatalf("outage did not preserve library: %+v, %v", unavailable, err)
	}
	if err := os.Rename(backup, root); err != nil {
		t.Fatal(err)
	}
	s.check(t.Context(), library.ID)
	available, err := s.Get(t.Context(), library.ID)
	if err != nil || available.Availability != "available" || available.ID != library.ID {
		t.Fatalf("recovery failed: %+v, %v", available, err)
	}
}

func TestStalledChecksDoNotBlockCRUDOrWriteLateResults(t *testing.T) {
	s := testService(t)
	library, err := s.Create(t.Context(), "NAS", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.timeout = 20 * time.Millisecond
	blocked := make(chan struct{})
	finished := make(chan struct{})
	s.probe.inspect = func(path string) error {
		<-blocked
		defer close(finished)
		return nil
	}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(blocked) }) })
	s.check(t.Context(), library.ID)
	row, err := s.Get(t.Context(), library.ID)
	if err != nil || row.Availability != "unavailable" {
		t.Fatalf("deadline did not report unavailable: %+v %v", row, err)
	}
	if _, err := s.List(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(t.Context(), library.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), library.ID); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(blocked) })
	<-finished
	if _, err := s.Get(t.Context(), library.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("late probe restored deleted library: %v", err)
	}
}

func TestRegistrationTimeoutNeverSavesLateResult(t *testing.T) {
	s := testService(t)
	s.timeout = 20 * time.Millisecond
	blocked := make(chan struct{})
	finished := make(chan struct{})
	s.probe.inspect = func(path string) error {
		<-blocked
		defer close(finished)
		return nil
	}
	_, err := s.Create(t.Context(), "NAS", t.TempDir())
	close(blocked)
	<-finished
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
	rows, err := s.List(t.Context())
	if err != nil || len(rows) != 0 {
		t.Fatalf("timed-out registration persisted: %+v, %v", rows, err)
	}
}

func TestFilesystemProbesBoundWorkAndShareStalledPaths(t *testing.T) {
	p := newFilesystemProbe(os.DirFS)
	blocked := make(chan struct{})
	var started atomic.Int32
	finished := make(chan struct{}, 20)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(blocked) }) })
	p.inspect = func(path string) error {
		started.Add(1)
		<-blocked
		finished <- struct{}{}
		return nil
	}
	// Repeated requests for one stalled path must consume only one slot.
	for range 3 {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		err := p.check(ctx, "same")
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	}
	if started.Load() != 1 {
		t.Fatalf("stalled path started %d probes", started.Load())
	}
	var callers sync.WaitGroup
	for i := range 20 {
		callers.Go(func() {
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			_ = p.check(ctx, string(rune('A'+i)))
		})
	}
	callers.Wait()
	if started.Load() != maxFilesystemProbes {
		t.Errorf("started %d probes, want bound %d", started.Load(), maxFilesystemProbes)
	}
	release.Do(func() { close(blocked) })
	for range maxFilesystemProbes {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("released probe did not finish")
		}
	}
}

func TestShutdownDoesNotWaitForStalledFilesystem(t *testing.T) {
	s := testService(t)
	if _, err := s.Create(t.Context(), "NAS", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })
	s.probe.inspect = func(path string) error {
		close(started)
		<-blocked
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	s.cancel = cancel
	s.start(ctx, time.Minute)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("startup check did not start")
	}
	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked on filesystem call")
	}
}

func TestScheduledAndExplicitChecks(t *testing.T) {
	s := testService(t)
	registered, err := s.Create(t.Context(), "NAS", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	checks := make(chan struct{}, 10)
	s.probe.inspect = func(path string) error {
		checks <- struct{}{}
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	s.cancel = cancel
	s.start(ctx, 50*time.Millisecond)
	await := func() {
		t.Helper()
		select {
		case <-checks:
		case <-time.After(2 * time.Second):
			t.Fatal("expected background availability check")
		}
	}
	await() // Startup.
	await() // Periodic.
	// Wait for the periodic check to leave the pending set before refreshing.
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		pending := s.pending[registered.ID]
		s.mu.Unlock()
		if !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("check did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if err := s.RequestCheck(t.Context(), registered.ID); err != nil {
		t.Fatal(err)
	}
	await()
	if err := s.RequestCheck(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing root accepted: %v", err)
	}
}
