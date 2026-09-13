CREATE TABLE panda_download_storage (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    reason TEXT NOT NULL DEFAULT '' CHECK (reason IN ('', 'low_space', 'space_check_failed')),
    archive_bytes INTEGER NOT NULL DEFAULT 0 CHECK (archive_bytes >= 0),
    gallery_id INTEGER NOT NULL DEFAULT 0
);

INSERT INTO panda_download_storage (id) VALUES (1);

ALTER TABLE panda_downloads ADD COLUMN expected_size_bytes INTEGER NOT NULL DEFAULT 0 CHECK (expected_size_bytes >= 0);
