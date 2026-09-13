-- Claims skip galleries already reserved by other metadata workers. Checking
-- older tokens must seek within one gallery, even when many proxies reserve
-- a long prefix of the pending import.
CREATE INDEX reference_import_entries_pending_gallery
ON reference_import_entries(import_id, gallery_id, id) WHERE status = 'pending';
