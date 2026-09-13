-- Existing pending entries need one resumable inventory check on upgrade.
-- New entries already check inventory while parsing or retrying.
CREATE TABLE reference_import_reconciliation (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    after_entry_id INTEGER NOT NULL,
    through_entry_id INTEGER NOT NULL
);
INSERT INTO reference_import_reconciliation
SELECT 1, 0, coalesce(max(id), 0) FROM reference_import_entries;

CREATE TABLE reference_import_inventory_queue (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    gallery_id INTEGER NOT NULL UNIQUE REFERENCES gallery_refs(gallery_id)
);

-- Every inventory writer participates without doing import updates in its
-- transaction. The pending-reference index makes the existence check bounded.
CREATE TRIGGER reference_import_inventory_insert AFTER INSERT ON gallery_refs
WHEN EXISTS (SELECT 1 FROM reference_import_entries WHERE gallery_id = NEW.gallery_id AND status = 'pending')
BEGIN
    INSERT INTO reference_import_inventory_queue (gallery_id) VALUES (NEW.gallery_id)
    ON CONFLICT (gallery_id) DO NOTHING;
END;
