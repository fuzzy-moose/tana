package downloads

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector/downloads/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestClaimSkipsJobsHeldByAnotherClaim(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx := t.Context()
	created := time.Now().Add(-time.Minute)
	for _, id := range []int64{1, 2} {
		if _, err := db.Exec(ctx, `INSERT INTO panda_downloads (gallery_id, token, state, created_at, updated_at)
			VALUES ($1, 'token', 'queued', $2, $2)`, id, created.Add(time.Duration(id)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	claim := func() (pgx.Tx, int64, error) {
		tx := pgtest.Begin(t, db)
		row, err := claimDownload(ctx, dbgen.New(tx), time.Now(), func(dbgen.PandaDownload) error { return nil })
		return tx, row.GalleryID, err
	}
	first, firstID, err := claim()
	if err != nil || firstID != 1 {
		t.Fatalf("first claim = %d, %v", firstID, err)
	}
	second, secondID, err := claim()
	if err != nil || secondID != 2 {
		t.Fatalf("second claim = %d, %v", secondID, err)
	}
	if _, id, err := claim(); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("third claim = %d, %v", id, err)
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A rolled-back claim returns its job to the queue; a committed one stays running.
	if err := second.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, id, err := claim(); err != nil || id != 2 {
		t.Fatalf("claim after rollback = %d, %v", id, err)
	}
}
