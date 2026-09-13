-- Total inventory counts need only a compact index, not reference contents.
CREATE INDEX gallery_refs_inventory ON gallery_refs(gallery_id);

-- Diagnostics must not scan successful collection history to find ten errors.
CREATE INDEX gallery_refs_recent_metadata_errors
ON gallery_refs(metadata_attempted_at DESC, gallery_id)
WHERE metadata_error IS NOT NULL AND metadata_error != '';
