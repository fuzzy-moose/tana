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
)

func testService(t *testing.T) *Service {
	t.Helper()
	s, err := openService(t.Context(), t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
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
	registered, err := s.Create(t.Context(), "  Manga  ", alias)
	if err != nil {
		t.Fatal(err)
	}
	if registered.Path != root || registered.Name != "Manga" || registered.Availability != "available" || registered.LastCheckedAt == nil {
		t.Fatalf("unexpected registration: %+v", registered)
	}
	for _, path := range []string{root, child, parent, alias, root + string(filepath.Separator)} {
		if _, err := s.Create(t.Context(), "Duplicate", path); !errors.Is(err, ErrRootConflict) {
			t.Errorf("path %q: want conflict, got %v", path, err)
		}
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

func TestConcurrentOverlappingRegistrations(t *testing.T) {
	s := testService(t)
	other, err := openService(t.Context(), s.dataDir, s.logger)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	parent := t.TempDir()
	child := filepath.Join(parent, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, service := range []*Service{s, other} {
		path := []string{parent, child}[i]
		go func() {
			<-start
			_, err := service.Create(t.Context(), "Library", path)
			results <- err
		}()
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRootConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d, conflicts=%d", successes, conflicts)
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
	s.probe.inspect = func(path string, canonicalize bool) (string, error) {
		<-blocked
		defer close(finished)
		return path, nil
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
	s.probe.inspect = func(path string, canonicalize bool) (string, error) {
		<-blocked
		defer close(finished)
		return path, nil
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
	p := newFilesystemProbe()
	blocked := make(chan struct{})
	var started atomic.Int32
	finished := make(chan struct{}, 20)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(blocked) }) })
	p.inspect = func(path string, canonicalize bool) (string, error) {
		started.Add(1)
		<-blocked
		finished <- struct{}{}
		return path, nil
	}
	// Repeated requests for one stalled path must consume only one slot.
	for range 3 {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		_, err := p.check(ctx, "same", false)
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
			_, _ = p.check(ctx, string(rune('A'+i)), false)
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
	s.probe.inspect = func(path string, canonicalize bool) (string, error) {
		close(started)
		<-blocked
		return path, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	s.cancel = cancel
	s.start(ctx, time.Minute)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("startup check did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
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
	s.probe.inspect = func(path string, canonicalize bool) (string, error) {
		checks <- struct{}{}
		return path, nil
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
