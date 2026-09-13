package metadata

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestReferenceImportsReconcileWithoutValidationCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openDB(t, t.TempDir())
		imports := batchImports(t, db, t.TempDir())
		item := acceptImport(t, imports, "1,token1\n2,conflict\n")
		unmatched := acceptImport(t, imports, "3,token3\n")
		cancelled := acceptImport(t, imports, "1,token1\n")
		parseImports(t, imports)
		before, err := imports.Cancel(t.Context(), cancelled.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := imports.Pause(t.Context(), item.ID); err != nil {
			t.Fatal(err)
		}
		// Another collection path populates inventory, with metadata still pending.
		seedRefs(t, db, 1, 2, 99)
		worker, err := NewReferenceImports(t.Context(), db, imports.dir, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		defer worker.Close()
		synctest.Wait()
		if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Paused || got.Known != 1 || got.Failed != 1 || got.Pending != 0 || got.CompletedAt == nil {
			t.Fatalf("inventory reconciliation waited for validation: %+v", got)
		}
		if got := readImport(t, imports, unmatched.ID); got.Pending != 1 {
			t.Fatalf("unmatched reference settled: %+v", got)
		}
		if got := readImport(t, imports, cancelled.ID); !reflect.DeepEqual(got, before) {
			t.Fatalf("reconciliation changed cancellation: %+v", got)
		}
		s := batchService(t, db, nil)
		refs, err := s.store.pendingRefs(t.Context())
		if err != nil || !reflect.DeepEqual(refs, []panda.GalleryRef{{ID: 1, Token: "token1"}, {ID: 2, Token: "token2"}, {ID: 99, Token: "token99"}}) {
			t.Fatalf("inventory work no longer outstanding: %v, %v", refs, err)
		}
		if _, err := db.Exec(`UPDATE gallery_refs SET metadata_attempted_at = 1`); err != nil {
			t.Fatal(err)
		}
		refs, err = s.store.pendingRefs(t.Context())
		if err != nil || !reflect.DeepEqual(refs, []panda.GalleryRef{{ID: 3, Token: "token3"}}) {
			t.Fatalf("settled imports scheduled redundant validation: %v, %v", refs, err)
		}
	})
}

func TestReferenceImportReconciliationBoundsFanoutAndCommitsCompletion(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir)
	imports := batchImports(t, db, filepath.Join(dir, "imports"))
	var input strings.Builder
	input.WriteString("1,token1\n")
	for i := range 2 * importBatchRecords {
		fmt.Fprintf(&input, "1,conflict%d\n", i)
	}
	item := acceptImport(t, imports, input.String())
	parseImports(t, imports)
	seedRefs(t, db, 1)
	if worked, err := imports.step(t.Context()); err != nil || !worked {
		t.Fatalf("reconcile first batch: %v, %v", worked, err)
	}
	got := readImport(t, imports, item.ID)
	if got.Pending != importBatchRecords+1 || got.Known+got.Failed != importBatchRecords || got.Status != "validating" {
		t.Fatalf("single-gallery reconciliation exceeded batch: %+v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, dir)
	imports = batchImports(t, db, imports.dir)
	if worked, err := imports.step(t.Context()); err != nil || !worked {
		t.Fatalf("resume queue: %v, %v", worked, err)
	}
	before := readImport(t, imports, item.ID)
	if before.Pending != 1 {
		t.Fatalf("reconciliation did not resume bounded progress: %+v", before)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_reconciled_completion BEFORE UPDATE OF status ON reference_imports
		WHEN NEW.status = 'completed' BEGIN SELECT RAISE(FAIL, 'storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.step(t.Context()); err == nil {
		t.Fatal("reconciliation unexpectedly committed")
	}
	if got := readImport(t, imports, item.ID); !reflect.DeepEqual(got, before) {
		t.Fatalf("partial reconciliation escaped rollback: %+v", got)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_reconciled_completion`); err != nil {
		t.Fatal(err)
	}
	parseImports(t, imports)
	if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Known != 1 || got.Failed != 2*importBatchRecords || got.Pending != 0 || got.CompletedAt == nil {
		t.Fatalf("reconciliation did not recover: %+v", got)
	}
	// Retry preserves conflict outcomes and reuses the immutable inventory token.
	if got, err := imports.Retry(t.Context(), item.ID); err != nil || got.Status != "completed" || got.Known != 1 || got.Failed != 2*importBatchRecords || got.Pending != 0 {
		t.Fatalf("retry changed inventory outcomes: %+v, %v", got, err)
	}
}

func TestReferenceImportReconciliationUpgradeResumesPastUnmatchedEntries(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir)
	imports := batchImports(t, db, filepath.Join(dir, "imports"))
	var input strings.Builder
	const count = 3*importBatchRecords + 1
	for id := 1; id <= count; id++ {
		fmt.Fprintf(&input, "%d,token%d\n", id, id)
	}
	item := acceptImport(t, imports, input.String())
	parseImports(t, imports)
	// Recreate the pre-migration state: inventory already exists but its import
	// entries are still pending, without a queued insertion event.
	if _, err := db.Exec(`DROP TRIGGER reference_import_inventory_insert;
		DROP TABLE reference_import_inventory_queue;
		DROP TABLE reference_import_reconciliation;
		DROP INDEX reference_import_entries_pending_gallery;
		DROP INDEX gallery_refs_inventory;
		DROP INDEX gallery_refs_recent_metadata_errors;
		ALTER TABLE reference_imports DROP COLUMN paused;
		CREATE TABLE panda_ban_legacy (id INTEGER PRIMARY KEY CHECK (id = 1), until_at INTEGER NOT NULL DEFAULT 0);
		INSERT INTO panda_ban_legacy SELECT id, until_at FROM panda_ban WHERE id = 1;
		DROP TABLE panda_ban;
		ALTER TABLE panda_ban_legacy RENAME TO panda_ban;
		PRAGMA user_version = 15`); err != nil {
		t.Fatal(err)
	}
	seedRefs(t, db, count)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, dir)
	imports = batchImports(t, db, imports.dir)
	if worked, err := imports.step(t.Context()); err != nil || !worked {
		t.Fatalf("first legacy batch: %v, %v", worked, err)
	}
	if got := readImport(t, imports, item.ID); got.Pending != count {
		t.Fatalf("legacy scan exceeded its entry budget to find a match: %+v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, dir)
	imports = batchImports(t, db, imports.dir)
	var after, through int64
	if err := db.QueryRow(`SELECT after_entry_id, through_entry_id FROM reference_import_reconciliation`).Scan(&after, &through); err != nil || after != importBatchRecords || through != count {
		t.Fatalf("legacy checkpoint lost on restart: %d through %d, %v", after, through, err)
	}
	// Keep parsing and inventory collection active while the sparse legacy scan
	// advances. Discovery behind its cursor must also settle through the queue.
	upload := acceptImport(t, imports, strings.ReplaceAll(input.String(), ",token", ",conflict"))
	for id := 1; id <= 3; id++ {
		seedRefs(t, db, int64(id))
		if worked, err := imports.step(t.Context()); err != nil || !worked {
			t.Fatalf("legacy/queue/parser progress: %v, %v", worked, err)
		}
		got := readImport(t, imports, item.ID)
		wantKnown := int64(id)
		if id == 3 {
			wantKnown++ // The final legacy page reaches the pre-migration match.
		}
		if got.Known != wantKnown {
			t.Fatalf("continuous work starved reconciliation: %+v", got)
		}
		if got := readImport(t, imports, upload.ID); got.References != int64(id*importBatchRecords) {
			t.Fatalf("reconciliation starved parsing: %+v", got)
		}
	}
	if worked, err := imports.reconcileInventory(t.Context()); err != nil || worked {
		t.Fatalf("revisited unmatched backlog after legacy scan: %v, %v", worked, err)
	}
	parseImports(t, imports)
}

func TestReferenceImportAdmissionBoundsConflictingTokens(t *testing.T) {
	db := openDB(t, t.TempDir())
	imports := batchImports(t, db, t.TempDir())
	var input strings.Builder
	input.WriteString("1,token1\n")
	for i := range importBatchRecords + 1 {
		fmt.Fprintf(&input, "1,conflict%d\n", i)
	}
	item := acceptImport(t, imports, input.String())
	parseImports(t, imports)
	s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1"}), nil
	}))
	collectBatch(t, s)
	if got := readImport(t, imports, item.ID); got.Imported != 1 || got.Failed != importBatchRecords || got.Pending != 1 {
		t.Fatalf("metadata admission exceeded reconciliation budget: %+v", got)
	}
	parseImports(t, imports)
	if got := readImport(t, imports, item.ID); got.Status != "completed" || got.Imported != 1 || got.Failed != importBatchRecords+1 || got.Pending != 0 {
		t.Fatalf("queued reconciliation lost admission outcome: %+v", got)
	}
}
