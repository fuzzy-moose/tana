package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestConcurrentOverlappingRegistrations(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	s := NewStore(db)
	parent := t.TempDir()
	child := filepath.Join(parent, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, path := range []string{parent, child} {
		go func() {
			<-start
			_, err := s.Create(t.Context(), "Library", path)
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
