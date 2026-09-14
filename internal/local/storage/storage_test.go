package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabasePersistsAndMigratesOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data #? with spaces")
	db, _, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = db.QueryRow("INSERT INTO libraries (name, path) VALUES (?, ?) RETURNING id", "Persistent", t.TempDir()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("first library ID: %d", id)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRow("SELECT name FROM libraries WHERE id = ?", id).Scan(&name); err != nil || name != "Persistent" {
		t.Fatalf("registration did not survive reopen: %q %v", name, err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version %d: %v", version, err)
	}
}

func TestFutureDatabaseVersionRejected(t *testing.T) {
	db, _, err := Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), db); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("future schema accepted: %v", err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 999 {
		t.Fatalf("future schema was changed: %d, %v", version, err)
	}
}

func TestMigrationCleansUpFinishedDeliveriesAndKeepsOutstandingWork(t *testing.T) {
	for _, tc := range []struct {
		batchState string
		itemState  string
		retained   int
	}{
		{"completed", "completed", 0},
		{"completed", "skipped", 0},
		{"stopped", "completed", 0},
		{"stopped", "queued", 1},
		{"completed_with_errors", "failed", 1},
		{"completed_with_errors", "cleanup_pending", 1},
		{"running", "queued", 1},
		{"paused", "queued", 1},
	} {
		t.Run(tc.batchState+"/"+tc.itemState, func(t *testing.T) {
			dir := t.TempDir()
			db, _, err := Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("INSERT INTO panda_deliveries (id, state, data) VALUES (1, ?, '{}')", tc.batchState); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO panda_delivery_items (delivery_id, position, gallery_id, state, cleanup_attempts, next_cleanup, data)
				VALUES (1, 0, 7, ?, 0, 0, '{}')`, tc.itemState); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("DROP TABLE panda_catalog_default_filter"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("DROP INDEX galleries_title"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("PRAGMA user_version = 4"); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				migrated, _, err := Open(t.Context(), dir)
				if err != nil {
					t.Fatal(err)
				}
				var batches, items int
				err = migrated.QueryRow(`SELECT (SELECT count(*) FROM panda_deliveries),
					(SELECT count(*) FROM panda_delivery_items)`).Scan(&batches, &items)
				if closeErr := migrated.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				if err != nil || batches != tc.retained || items != tc.retained {
					t.Fatalf("retained batches=%d items=%d, want %d: %v", batches, items, tc.retained, err)
				}
			}
		})
	}
}
