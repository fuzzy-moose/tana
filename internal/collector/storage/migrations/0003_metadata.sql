ALTER TABLE gallery_refs ADD COLUMN metadata_attempted_at INTEGER;
ALTER TABLE gallery_refs ADD COLUMN metadata_error TEXT;
CREATE INDEX gallery_refs_pending_metadata ON gallery_refs(gallery_id) WHERE metadata_attempted_at IS NULL;

CREATE TABLE gallery_metadata (
    gallery_id INTEGER PRIMARY KEY REFERENCES gallery_refs(gallery_id),
    body BLOB NOT NULL,
    refreshed_at INTEGER NOT NULL
);

CREATE TABLE metadata_retry (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    failures INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL DEFAULT 0,
    last_attempt_at INTEGER,
    last_error TEXT
);

INSERT INTO metadata_retry (id) VALUES (1);
