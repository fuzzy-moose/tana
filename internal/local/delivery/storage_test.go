package delivery

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func TestDeliveryCheckpointWritesHaveBoundedSize(t *testing.T) {
	f := newFixture(t)
	ids := make([]int64, 10000)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	f.start(ids...)
	if _, err := f.db.Exec("CREATE TEMP TABLE checkpoint_writes (bytes INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	rows, err := f.db.Query("SELECT name FROM sqlite_schema WHERE type = 'table' AND name IN ('panda_deliveries', 'panda_delivery_items')")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		query := fmt.Sprintf("CREATE TEMP TRIGGER audit_%s AFTER UPDATE ON %s BEGIN INSERT INTO checkpoint_writes VALUES (length(NEW.data)); END", table, table)
		if _, err := f.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if more, err := f.service.step(); err != nil || !more {
		t.Fatalf("advance archive: more=%v, err=%v", more, err)
	}
	var writes, bytes int
	if err := f.db.QueryRow("SELECT count(*), coalesce(sum(bytes), 0) FROM checkpoint_writes").Scan(&writes, &bytes); err != nil {
		t.Fatal(err)
	}
	t.Logf("one archive in a 10,000-item batch: %d JSON bytes in %d writes", bytes, writes)
	if writes == 0 || bytes > 16*1024 {
		t.Fatalf("checkpoint writes must not rewrite the 10,000-item batch: %d bytes in %d writes", bytes, writes)
	}
}

func TestDeliveryCheckpointUpdatesAreAtomic(t *testing.T) {
	for _, update := range []string{"worker", "batch action"} {
		t.Run(update, func(t *testing.T) {
			f := newFixture(t)
			b := f.start(7, 8)
			if _, err := f.db.Exec("CREATE TRIGGER reject_metadata BEFORE UPDATE ON panda_deliveries BEGIN SELECT RAISE(ABORT, 'metadata write failed'); END"); err != nil {
				t.Fatal(err)
			}
			var err error
			if update == "worker" {
				item := b.Items[0]
				item.State, item.checkpoint.Stage = "saved", "saved"
				err = f.service.updateItem(b, item)
			} else {
				_, err = f.service.change(t.Context(), b.ID, func(current *Batch) error {
					for i := range current.Items {
						current.Items[i].State = "failed"
					}
					return nil
				})
			}
			if err == nil {
				t.Fatal("expected rejected metadata write")
			}
			if got := f.get(b.ID); !reflect.DeepEqual(got, b) {
				t.Fatalf("failed transaction changed durable delivery: got %+v, want %+v", got, b)
			}
		})
	}
}

func TestDeliveryCheckpointsMigrateFromBatchJSON(t *testing.T) {
	dir := t.TempDir()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	want := Batch{
		ID: 12, LibraryID: 3, State: "stopped", StopRequested: true,
		CurrentGalleryID: 7, Error: "destination unavailable", root: "/library",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
		Items: []Item{
			{GalleryID: 9, State: "queued", checkpoint: checkpoint{Token: "queued-token", Stage: "queued"}},
			{GalleryID: 7, State: "failed", Error: "import failed", checkpoint: checkpoint{Token: "saved-token", Stage: "saved", SHA256: "archive-hash", Size: 42}},
			{GalleryID: 8, State: "cleanup_pending", CleanupAttempts: 2, Error: "collector unavailable", checkpoint: checkpoint{Token: "cleanup-token", Stage: "cleanup_pending", SHA256: "other-hash", Size: 56, NextCleanup: now.Add(time.Minute)}},
		},
	}
	legacy := struct {
		Batch       Batch        `json:"batch"`
		Root        string       `json:"root"`
		Checkpoints []checkpoint `json:"checkpoints"`
	}{Batch: want, Root: want.root}
	for _, item := range want.Items {
		legacy.Checkpoints = append(legacy.Checkpoints, item.checkpoint)
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE panda_delivery_items"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO panda_deliveries (id, state, data) VALUES (?, ?, ?)", want.ID, want.State, data); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Service{db: db}
	got, err := s.Get(t.Context(), want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migration changed delivery: got %+v, want %+v", got, want)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM panda_delivery_items WHERE delivery_id = ?", want.ID).Scan(&count); err != nil || count != len(want.Items) {
		t.Fatalf("independent checkpoints: count=%d, err=%v", count, err)
	}
	if err := db.QueryRow("SELECT data FROM panda_deliveries WHERE id = ?", want.ID).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if _, exists := record["checkpoints"]; exists {
		t.Fatal("checkpoints remain embedded in the batch record")
	}
}
