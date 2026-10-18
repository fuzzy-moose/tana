package delivery

import (
	"reflect"
	"testing"
)

func TestDeliveryCheckpointWritesHaveBoundedSize(t *testing.T) {
	f := newFixture(t)
	ids := make([]int64, 10000)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	f.start(ids...)
	if _, err := f.db.Exec(t.Context(), `CREATE TABLE checkpoint_writes (bytes INTEGER NOT NULL);
		CREATE FUNCTION audit_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN INSERT INTO checkpoint_writes VALUES (octet_length(NEW.data::text)); RETURN NEW; END $$;
		CREATE TRIGGER audit_checkpoint AFTER UPDATE ON panda_deliveries FOR EACH ROW EXECUTE FUNCTION audit_checkpoint();
		CREATE TRIGGER audit_checkpoint AFTER UPDATE ON panda_delivery_items FOR EACH ROW EXECUTE FUNCTION audit_checkpoint()`); err != nil {
		t.Fatal(err)
	}
	if more, err := f.service.step(); err != nil || !more {
		t.Fatalf("advance archive: more=%v, err=%v", more, err)
	}
	var writes, bytes int
	if err := f.db.QueryRow(t.Context(), "SELECT count(*), coalesce(sum(bytes), 0) FROM checkpoint_writes").Scan(&writes, &bytes); err != nil {
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
			if _, err := f.db.Exec(t.Context(), `CREATE FUNCTION reject_metadata() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'metadata write failed'; END $$;
				CREATE TRIGGER reject_metadata BEFORE UPDATE ON panda_deliveries FOR EACH ROW EXECUTE FUNCTION reject_metadata()`); err != nil {
				t.Fatal(err)
			}
			var err error
			if update == "worker" {
				item := b.Items[0]
				item.State, item.checkpoint.Stage = "saved", "saved"
				err = f.service.updateItem(b, "queued", item)
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
