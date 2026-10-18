package delivery

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestBatchHeldByAnotherTransactionIsNotAdvanced(t *testing.T) {
	f := newFixture(t)
	b := f.start(7)
	held := pgtest.Begin(t, f.db)
	if _, err := held.Exec(t.Context(), "SELECT 1 FROM panda_deliveries WHERE id = $1 FOR UPDATE", b.ID); err != nil {
		t.Fatal(err)
	}
	if more, err := f.service.step(); err != nil || more {
		t.Fatalf("step past a held batch: more=%v, err=%v", more, err)
	}
	if got := f.get(b.ID); got.CurrentGalleryID != 0 || len(f.events) != 0 {
		t.Fatalf("held batch advanced: %+v, events=%v", got, f.events)
	}
	if err := held.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if more, err := f.service.step(); err != nil || !more || len(f.events) == 0 || f.events[0] != "transfer:7" {
		t.Fatalf("released batch: more=%v, err=%v, events=%v", more, err, f.events)
	}
}

func TestItemClaimsNeverShareAnItem(t *testing.T) {
	f := newFixture(t)
	b := f.start(7, 8)
	claim := func() (pgx.Tx, Batch, claimResult) {
		t.Helper()
		tx := pgtest.Begin(t, f.db)
		claimed, result, err := claimNext(t.Context(), tx)
		if err != nil {
			t.Fatal(err)
		}
		return tx, claimed, result
	}
	first, claimed, result := claim()
	if result != claimItem || claimed.Items[0].GalleryID != 7 || claimed.Items[0].State != "transferring" {
		t.Fatalf("first claim: %v %+v", result, claimed)
	}
	if _, other, result := claim(); result != claimNone {
		t.Fatalf("second claim during the first: %v %+v", result, other)
	}
	if err := first.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A transition from a state the claim already left is lost.
	stale := b.Items[0]
	stale.State = "skipped"
	if err := f.service.updateItem(b, "queued", stale); !errors.Is(err, errItemChanged) {
		t.Fatalf("stale transition: %v", err)
	}
	if got := f.get(b.ID); got.Items[0].State != "transferring" || got.CurrentGalleryID != 7 {
		t.Fatalf("after stale transition: %+v", got)
	}
	// A claimer never skips past an item another transaction holds.
	held := pgtest.Begin(t, f.db)
	if _, err := held.Exec(t.Context(), "SELECT 1 FROM panda_delivery_items WHERE delivery_id = $1 AND gallery_id = 7 FOR UPDATE", b.ID); err != nil {
		t.Fatal(err)
	}
	if _, other, result := claim(); result != claimNone {
		t.Fatalf("claim past a held item: %v %+v", result, other)
	}
}

func TestCleanupHeldByAnotherTransactionIsNotAttempted(t *testing.T) {
	f := newFixture(t)
	f.collector.deleteError[7] = errors.New("collector unavailable")
	b := f.drain(f.start(7).ID)
	if b.Items[0].State != "cleanup_pending" || b.Items[0].CleanupAttempts != 1 {
		t.Fatalf("batch: %+v", b)
	}
	delete(f.collector.deleteError, 7)
	held := pgtest.Begin(t, f.db)
	if _, err := held.Exec(t.Context(), "SELECT 1 FROM panda_delivery_items WHERE delivery_id = $1 AND gallery_id = 7 FOR UPDATE", b.ID); err != nil {
		t.Fatal(err)
	}
	before := len(f.events)
	if err := f.service.cleanup(b, 0); err != nil {
		t.Fatal(err)
	}
	if got := f.get(b.ID); len(f.events) != before || got.Items[0].CleanupAttempts != 1 {
		t.Fatalf("held cleanup attempted: %+v, events=%v", got, f.events[before:])
	}
	if err := held.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.service.cleanup(b, 0); err != nil {
		t.Fatal(err)
	}
	if len(f.events) != before+1 || f.events[before] != "delete:7" {
		t.Fatalf("released cleanup: events=%v", f.events[before:])
	}
	f.assertRemoved(b.ID)
}
