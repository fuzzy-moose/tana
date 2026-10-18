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
	db := openDB(t)
	const reserved = 4000
	_, err := db.Exec(t.Context(), `INSERT INTO reference_imports (id, filename, status, created_at, size_bytes)
		VALUES ('large', 'large.txt', 'validating', to_timestamp(1), 0);
		INSERT INTO reference_import_entries (import_id, gallery_id, token, status)
		SELECT 'large', id, 'token', 'pending' FROM generate_series(1, 5000) AS id`)
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
	view := status.New(db, nil, pandaban.New(db), pandaban.NewAuthenticated(db), nil, nil, nil)
	if _, err := view.Metadata(ctx); err != nil {
		t.Fatalf("metadata status after import claim: %v", err)
	}
	t.Logf("claim and status: %s", time.Since(start))
}
