-- Inventory needs only membership, not retained metadata bodies. Keep those
-- lookups in a compact covering index so large collections do not repeatedly
-- read the metadata table while holding the shared database connection.
CREATE INDEX gallery_metadata_inventory ON gallery_metadata(gallery_id);
