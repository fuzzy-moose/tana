package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestStatusDuringLargeImportClaims(t *testing.T) {
	db := openDB(t, t.TempDir())
	const reserved = 4000
	_, err := db.Exec(`INSERT INTO reference_imports (id, filename, status, created_at, size_bytes)
		VALUES ('large', 'large.txt', 'validating', 1, 0);
		WITH RECURSIVE ids(id) AS (VALUES(1) UNION ALL SELECT id + 1 FROM ids WHERE id < 5000)
		INSERT INTO reference_import_entries (import_id, gallery_id, token, status)
		SELECT 'large', id, 'token', 'pending' FROM ids`)
	if err != nil {
		t.Fatal(err)
	}
	s := batchService(t, db, nil)
	s.reserved = make(map[int64]bool, reserved)
	for id := int64(1); id <= reserved; id++ {
		s.reserved[id] = true
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	batch, err := s.ClaimBackground(ctx)
	if err != nil {
		t.Fatalf("import claim held the status database too long: %v (%s)", err, time.Since(start))
	}
	if batch == nil || batch.Size() != panda.MaxBatchSize || batch.refs[0].ID != reserved+1 {
		t.Fatalf("unreserved import batch: %+v", batch)
	}
	defer batch.Close()
	view := status.New(db, nil, pandaban.New(db))
	if _, err := view.Metadata(ctx); err != nil {
		t.Fatalf("metadata status after import claim: %v", err)
	}
	t.Logf("claim and status: %s", time.Since(start))
}
