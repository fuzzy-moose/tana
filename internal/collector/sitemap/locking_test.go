package sitemap

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector/sitemap/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestChildClaimSkipsChildrenHeldByAnotherClaim(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx := t.Context()
	for _, url := range []string{"https://panda.test/1.xml", "https://panda.test/2.xml"} {
		if err := dbgen.New(db).AddChild(ctx, url); err != nil {
			t.Fatal(err)
		}
	}
	claim := func() (int64, error) {
		q := dbgen.New(pgtest.Begin(t, db))
		child, err := q.ClaimChild(ctx)
		if err != nil {
			return 0, err
		}
		if started, err := q.StartChild(ctx, child.ID); err != nil || started != 1 {
			t.Fatalf("start child %d = %d, %v", child.ID, started, err)
		}
		return child.ID, nil
	}
	first, err := claim()
	if err != nil {
		t.Fatal(err)
	}
	second, err := claim()
	if err != nil || second == first {
		t.Fatalf("second claim = %d (first %d), %v", second, first, err)
	}
	if id, err := claim(); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("third claim = %d, %v", id, err)
	}
}
