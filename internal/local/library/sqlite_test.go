package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func TestConcurrentOverlappingRegistrations(t *testing.T) {
	dir := t.TempDir()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	otherDB, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	s, other := NewSQLiteRepository(db), NewSQLiteRepository(otherDB)
	parent := t.TempDir()
	child := filepath.Join(parent, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, service := range []Repository{s, other} {
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
