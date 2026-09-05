package library

import (
	"errors"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryDeletionCascadesToOwnedRecords(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteRepository(db)
	root, err := s.Create(t.Context(), "Comics", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the relationship required of future catalog/progress tables.
	_, err = db.Exec(`CREATE TABLE owned_records (
		library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		value TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO owned_records VALUES (?, 'progress')", root.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), root.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM owned_records").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned records=%d: %v", count, err)
	}
	if _, err := db.Exec("INSERT INTO owned_records VALUES ('missing', 'progress')"); err == nil {
		t.Fatal("foreign key constraint not enforced")
	}
}

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
