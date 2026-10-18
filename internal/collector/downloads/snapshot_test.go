package downloads

import (
	"slices"
	"testing"
	"time"
)

func TestCompletedDownloadIDsIncludesWholeBacklog(t *testing.T) {
	db := openDB(t)
	s := &Service{db: db}
	ids, err := s.CompletedDownloadIDs(t.Context())
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("empty snapshot: %v, %v", ids, err)
	}
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	want := make([]int64, 105)
	for i := range want {
		id := int64(i + 1)
		// Two creation times exercise both ordering keys.
		if _, err := tx.Exec(t.Context(), `INSERT INTO panda_downloads
			(gallery_id, token, state, created_at, updated_at) VALUES ($1, 'secret', 'completed', $2, $3)`, id, time.UnixMilli(id/50), time.UnixMilli(1)); err != nil {
			t.Fatal(err)
		}
		want[len(want)-i-1] = id
	}
	for i, state := range []string{"queued", "running", "failed", "cancelled", "deleting"} {
		if _, err := tx.Exec(t.Context(), `INSERT INTO panda_downloads
			(gallery_id, token, state, created_at, updated_at) VALUES ($1, 'secret', $2, $3, $4)`, i+200, state, time.UnixMilli(100), time.UnixMilli(1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	ids, err = s.CompletedDownloadIDs(t.Context())
	if err != nil || !slices.Equal(ids, want) {
		t.Fatalf("completed snapshot: %v, %v", ids, err)
	}
}
