ALTER TABLE metadata_proxy_settings ADD COLUMN auto_remove_inactive INTEGER NOT NULL DEFAULT 0 CHECK (auto_remove_inactive IN (0, 1));

ALTER TABLE metadata_proxy_channels ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0;
-- Existing channels without a success get the same initial grace as new ones.
UPDATE metadata_proxy_channels SET created_at = CAST(unixepoch('subsec') * 1000 AS INTEGER);
