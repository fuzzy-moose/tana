ALTER TABLE metadata_collection_settings ADD COLUMN feed_metadata_delay_hours INTEGER NOT NULL DEFAULT 12 CHECK (feed_metadata_delay_hours BETWEEN 0 AND 8760);
