package favorites

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestSyncClaimSkipsSyncsHeldByAnotherClaim(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx := t.Context()
	s := &store{db: db, q: dbgen.New(db), host: "https://panda.test", accountKey: "42"}
	if err := s.transaction(ctx, func(q *dbgen.Queries) error { return s.enqueueCategories(ctx, q, []int{2, 5}, true) }); err != nil {
		t.Fatal(err)
	}
	claim := func() (int, error) {
		current, err := s.claim(ctx, s.q.WithTx(pgtest.Begin(t, db)))
		if err == nil && current.State != "running" {
			t.Fatalf("claimed sync in state %q", current.State)
		}
		return current.category, err
	}
	first, err := claim()
	if err != nil {
		t.Fatal(err)
	}
	second, err := claim()
	if err != nil || second == first {
		t.Fatalf("second claim = %d (first %d), %v", second, first, err)
	}
	if category, err := claim(); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("third claim = %d, %v", category, err)
	}
}
