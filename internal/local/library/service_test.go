package library

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Unused methods deliberately fail through the embedded interface. These tests
// exercise library decisions independently of SQLite and the host filesystem.
type stubRepository struct {
	Repository
	create            func(context.Context, string, string) (Library, error)
	get               func(context.Context, string) (Library, error)
	update            func(context.Context, string, string, time.Time) error
	listErr, resetErr error
}

func (r stubRepository) List(context.Context) ([]Library, error) {
	return []Library{}, r.listErr
}

func (r stubRepository) ResetAvailability(context.Context) error {
	return r.resetErr
}

func (r stubRepository) Create(ctx context.Context, name, path string) (Library, error) {
	return r.create(ctx, name, path)
}

func (r stubRepository) Get(ctx context.Context, id string) (Library, error) {
	return r.get(ctx, id)
}

func (r stubRepository) UpdateAvailability(ctx context.Context, id, availability string, at time.Time) error {
	return r.update(ctx, id, availability, at)
}

func TestRegistrationUsesRepositoryAfterCheckingRoot(t *testing.T) {
	root, err := filepath.Abs("virtual/comics")
	if err != nil {
		t.Fatal(err)
	}
	storage := filepath.Join(root, "..", "data")
	for _, failure := range []error{nil, ErrRootConflict, errors.New("storage failed")} {
		name := "success"
		if failure != nil {
			name = failure.Error()
		}
		t.Run(name, func(t *testing.T) {
			repository := stubRepository{create: func(ctx context.Context, name, path string) (Library, error) {
				if name != "Manga" || path != root {
					t.Errorf("unclean registration: %q %q", name, path)
				}
				return Library{ID: "registered", Name: name, Path: path}, failure
			}}
			s, err := New(t.Context(), repository, os.DirFS, storage, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.probe.inspect = func(path string) error {
				if path != root {
					t.Errorf("unclean probe path: %q", path)
				}
				return nil
			}
			got, err := s.Create(t.Context(), "  Manga  ", root+string(filepath.Separator)+".")
			if !errors.Is(err, failure) {
				t.Fatalf("got %v, want %v", err, failure)
			}
			if failure == nil && (got.ID != "registered" || got.Name != "Manga" || got.Path != root) {
				t.Fatalf("unexpected registration: %+v", got)
			}
		})
	}
}

func TestRegistrationRejectsInputBeforePersistence(t *testing.T) {
	root, err := filepath.Abs("virtual/comics")
	if err != nil {
		t.Fatal(err)
	}
	storage := filepath.Join(root, "..", "data")
	for _, tc := range []struct {
		name, path     string
		probeErr, want error
	}{
		{" ", root, nil, ErrInvalidName},
		{"Relative", "relative", nil, ErrInvalidPath},
		{"Missing", root, errors.New("offline"), ErrRootUnavailable},
		{"Storage", storage, nil, ErrStorageOverlap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := stubRepository{create: func(context.Context, string, string) (Library, error) {
				t.Error("invalid registration reached persistence")
				return Library{}, nil
			}}
			s, err := New(t.Context(), repository, os.DirFS, storage, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.probe.inspect = func(string) error { return tc.probeErr }
			if _, err := s.Create(t.Context(), tc.name, tc.path); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestStartupPropagatesRepositoryFailures(t *testing.T) {
	failure := errors.New("storage failed")
	for _, repository := range []stubRepository{{listErr: failure}, {resetErr: failure}} {
		s, err := New(t.Context(), repository, os.DirFS, "unused", slog.New(slog.NewTextHandler(io.Discard, nil)))
		if s != nil {
			s.Close()
			t.Fatal("failed startup returned a service")
		}
		if !errors.Is(err, failure) {
			t.Fatalf("lost repository error: %v", err)
		}
	}
}

type stubFS struct {
	fs.FS
	err error
}

func (f stubFS) Stat(string) (fs.FileInfo, error) {
	return nil, f.err
}

func TestRequestedCheckPersistsUnavailableObservation(t *testing.T) {
	observed := make(chan string, 1)
	repository := stubRepository{
		get: func(context.Context, string) (Library, error) {
			return Library{ID: "nas", Path: "unused"}, nil
		},
		update: func(ctx context.Context, id, availability string, at time.Time) error {
			if id != "nas" || at.IsZero() {
				t.Errorf("invalid observation: %q %v", id, at)
			}
			observed <- availability
			return nil
		},
	}
	dirFS := func(string) fs.FS { return stubFS{err: fs.ErrNotExist} }
	s, err := New(t.Context(), repository, dirFS, "unused", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RequestCheck(t.Context(), "nas"); err != nil {
		t.Fatal(err)
	}
	select {
	case availability := <-observed:
		if availability != "unavailable" {
			t.Fatalf("offline root became %q", availability)
		}
	case <-time.After(time.Second):
		t.Fatal("availability observation was not persisted")
	}
}
