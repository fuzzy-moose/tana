package downloads

import (
	"slices"
	"testing"
)

func TestCompletedDownloadIDsIncludesWholeBacklog(t *testing.T) {
	db := openDB(t, t.TempDir())
	s := &Service{db: db}
	ids, err := s.CompletedDownloadIDs(t.Context())
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("empty snapshot: %v, %v", ids, err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	want := make([]int64, 105)
	for i := range want {
		id := int64(i + 1)
		// Two creation times exercise both ordering keys.
		if _, err := tx.ExecContext(t.Context(), `INSERT INTO panda_downloads
			(gallery_id, token, state, created_at, updated_at) VALUES (?, 'secret', 'completed', ?, 1)`, id, id/50); err != nil {
			t.Fatal(err)
		}
		want[len(want)-i-1] = id
	}
	for i, state := range []string{"queued", "running", "failed", "cancelled", "deleting"} {
		if _, err := tx.ExecContext(t.Context(), `INSERT INTO panda_downloads
			(gallery_id, token, state, created_at, updated_at) VALUES (?, 'secret', ?, 100, 1)`, i+200, state); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	ids, err = s.CompletedDownloadIDs(t.Context())
	if err != nil || !slices.Equal(ids, want) {
		t.Fatalf("completed snapshot: %v, %v", ids, err)
	}
}
