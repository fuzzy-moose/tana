package refresh

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/refresh/dbgen"
)

type unreadableFS struct{ fs.FS }

func (f unreadableFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "private" {
		// Even partial results with an error cannot establish absence.
		return []fs.DirEntry{}, fs.ErrPermission
	}
	return fs.ReadDir(f.FS, name)
}

func TestRefreshDistinguishesMissingPathsFromUnreadableDirectories(t *testing.T) {
	files := fstest.MapFS{
		"present.zip":       {Data: []byte("present")},
		"empty":             {Mode: fs.ModeDir | 0o700},
		"private/known.cbz": {Data: []byte("present")},
		"changed/subdir":    {Mode: fs.ModeDir | 0o700},
		"file-parent":       {Data: []byte("no longer a directory")},
	}
	rows := []dbgen.ListSourcesRow{
		{ID: 1, Path: "present.zip"},
		{ID: 2, Path: "empty"},
		{ID: 3, Path: "private/known.cbz"},
		{ID: 4, Path: "private/missing.cbz"},
		{ID: 5, Path: "removed/volume.cbz"},
		{ID: 6, Path: "changed"},
		{ID: 7, Path: "file-parent/volume.cbz"},
		{ID: 8, Path: "../outside.cbz"},
		{ID: 9, Path: "."},
	}
	result := inspect(t.Context(), unreadableFS{files}, rows)
	if result.blocked != "" || len(result.missing) != 1 || !result.missing[5] {
		t.Fatalf("missing paths = %+v", result)
	}
	if len(result.uncertain) != 4 {
		t.Fatalf("uncertain paths = %+v", result)
	}
	for _, id := range []int64{3, 4, 7, 8} {
		if result.uncertain[id] == "" {
			t.Fatalf("source %d not preserved: %+v", id, result)
		}
	}
}

type stalledFS struct {
	fs.FS
	started chan struct{}
	release chan struct{}
}

func (f stalledFS) ReadDir(name string) ([]fs.DirEntry, error) {
	close(f.started)
	<-f.release
	return fs.ReadDir(f.FS, name)
}

func TestRefreshBoundsStalledStorageChecksAndDiscardsLateResults(t *testing.T) {
	files := stalledFS{FS: fstest.MapFS{}, started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(files.release) })
	s := New(nil, func(string) fs.FS { return files })
	s.probeSlots = make(chan struct{}, 1)
	s.probeTimeout = time.Second
	rows := []dbgen.ListSourcesRow{{ID: 1, LibraryPath: "/nas", Path: "Gone.zip"}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan observation, 1)
	go func() { done <- s.probe(ctx, rows) }()
	<-files.started
	cancel()
	if got := <-done; got.blocked == "" || len(got.missing) != 0 {
		t.Fatalf("canceled probe supplied removal evidence: %+v", got)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if got := s.probe(ctx, rows); got.blocked == "" || len(got.missing) != 0 {
		t.Fatalf("stalled probe admitted more work: %+v", got)
	}
}
