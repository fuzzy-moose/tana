ALTER TABLE gallery_refs ADD COLUMN metadata_priority INTEGER NOT NULL DEFAULT 0;

DROP INDEX gallery_refs_pending_metadata;
CREATE INDEX gallery_refs_pending_metadata ON gallery_refs(metadata_priority, gallery_id)
WHERE metadata_attempted_at IS NULL;
