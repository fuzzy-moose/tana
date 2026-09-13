package delivery

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/library"
)

type stalledCatalog struct {
	lib     library.Library
	entered chan struct{}
	release chan struct{}
}

func (c stalledCatalog) Get(ctx context.Context, _ int64) (library.Library, error) {
	close(c.entered)
	select {
	case <-c.release:
		return c.lib, nil
	case <-ctx.Done():
		return library.Library{}, ctx.Err()
	}
}

func TestDestinationCheckDoesNotBlockBatchControls(t *testing.T) {
	for _, action := range []string{"resume", "retry"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			b := f.start(7)
			state := "paused"
			if action == "retry" {
				state = "stopped"
			}
			if _, err := f.service.change(t.Context(), b.ID, func(b *Batch) error { b.State = state; return nil }); err != nil {
				t.Fatal(err)
			}
			catalog := stalledCatalog{lib: f.lib, entered: make(chan struct{}), release: make(chan struct{})}
			f.service.libraries = catalog
			defer close(catalog.release)
			operation := f.service.Resume
			if action == "retry" {
				operation = f.service.Retry
			}
			done := make(chan error, 1)
			go func() { _, err := operation(t.Context(), b.ID); done <- err }()
			<-catalog.entered
			listed := make(chan error, 1)
			go func() { _, err := f.service.List(t.Context()); listed <- err }()
			select {
			case err := <-listed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("destination check blocked delivery listing")
			}
			// A user action during the probe must win over the stale snapshot.
			if action == "resume" {
				if _, err := f.service.Stop(t.Context(), b.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.service.change(t.Context(), b.ID, func(b *Batch) error { b.State = "completed"; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			catalog.release <- struct{}{}
			if err := <-done; !errors.Is(err, ErrInvalid) {
				t.Fatalf("stale %s: %v", action, err)
			}
		})
	}
}

type stalledDirectory struct {
	fs.FS
	entered chan struct{}
	release chan struct{}
}

func (d stalledDirectory) Stat(name string) (fs.FileInfo, error) {
	close(d.entered)
	<-d.release
	return fs.Stat(d.FS, name)
}

func TestDestinationProbeStopsWaiting(t *testing.T) {
	for _, reason := range []string{"deadline", "request cancellation", "shutdown"} {
		t.Run(reason, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFixture(t)
				b := f.start(7)
				if _, err := f.service.change(t.Context(), b.ID, func(b *Batch) error { b.State = "paused"; return nil }); err != nil {
					t.Fatal(err)
				}
				entered, release := make(chan struct{}), make(chan struct{})
				defer close(release)
				f.service.probe = library.NewDirectoryProbe(func(path string) fs.FS {
					return stalledDirectory{FS: os.DirFS(path), entered: entered, release: release}
				})
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() { _, err := f.service.Resume(ctx, b.ID); done <- err }()
				<-entered
				switch reason {
				case "request cancellation":
					cancel()
				case "shutdown":
					f.service.Close()
				}
				select {
				case err := <-done:
					if !errors.Is(err, ErrUnavailable) {
						t.Fatalf("probe: %v", err)
					}
				case <-time.After(2 * destinationProbeTimeout):
					t.Fatal("stalled filesystem probe kept caller blocked")
				}
				if current := f.get(b.ID); current.State != "paused" {
					t.Fatalf("batch resumed without available destination: %s", current.State)
				}
			})
		})
	}
}
