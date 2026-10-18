package metadata

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestFetchBatchClaimSkipsFetchesHeldByAnotherClaim(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx := t.Context()
	total := panda.MaxBatchSize + 5
	for id := 1; id <= total; id++ {
		if _, err := db.Exec(ctx, `INSERT INTO metadata_fetches (gallery_id, token, status) VALUES ($1, 'token', 'pending')`, id); err != nil {
			t.Fatal(err)
		}
	}
	s := &store{db: db, q: dbgen.New(db)}
	// Gallery 1 is reserved by a batch in flight: claims neither return nor lock it.
	reserved := map[int64]bool{1: true}
	claimed := make(map[int64]bool)
	for _, want := range []int{panda.MaxBatchSize, total - 1 - panda.MaxBatchSize, 0} {
		refs, err := s.claimRefs(ctx, pgtest.Begin(t, db), false, reserved)
		if err != nil || len(refs) != want {
			t.Fatalf("claimed %d refs, want %d: %v", len(refs), want, err)
		}
		for _, ref := range refs {
			if claimed[ref.ID] || reserved[ref.ID] {
				t.Fatalf("gallery %d claimed twice or while reserved", ref.ID)
			}
			claimed[ref.ID] = true
		}
	}
	if _, err := pgtest.Begin(t, db).Exec(ctx, `SELECT 1 FROM metadata_fetches WHERE gallery_id = 1 FOR UPDATE NOWAIT`); err != nil {
		t.Fatalf("reserved gallery is locked: %v", err)
	}
}

func TestInventoryQueueClaimSkipsGalleriesHeldByAnotherClaim(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx := t.Context()
	for id := 1; id <= 3; id++ {
		if _, err := db.Exec(ctx, `INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO reference_import_inventory_queue (gallery_id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pgtest.Begin(t, db).Exec(ctx, `SELECT 1 FROM reference_import_inventory_queue WHERE gallery_id = 1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	first, err := claimInventoryQueue(ctx, pgtest.Begin(t, db))
	if err != nil || len(first) != 2 || first[0].id != 2 || first[1].id != 3 {
		t.Fatalf("first claim = %+v, %v", first, err)
	}
	if second, err := claimInventoryQueue(ctx, pgtest.Begin(t, db)); err != nil || len(second) != 0 {
		t.Fatalf("second claim = %+v, %v", second, err)
	}
}
