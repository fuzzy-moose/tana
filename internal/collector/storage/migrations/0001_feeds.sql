CREATE TABLE raw_feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    captured_at INTEGER NOT NULL,
    feed_url TEXT NOT NULL,
    body BLOB NOT NULL,
    processed_at INTEGER,
    last_attempt_at INTEGER,
    last_error TEXT
);

CREATE INDEX raw_feeds_capture_order ON raw_feeds(captured_at, id);
CREATE INDEX raw_feeds_pending ON raw_feeds(captured_at, id) WHERE processed_at IS NULL;

CREATE TABLE gallery_refs (
    gallery_id INTEGER PRIMARY KEY CHECK (gallery_id > 0),
    token TEXT NOT NULL CHECK (length(token) > 0)
);

CREATE TABLE feed_continuity_checks (
    previous_capture_id INTEGER NOT NULL REFERENCES raw_feeds(id),
    current_capture_id INTEGER NOT NULL REFERENCES raw_feeds(id),
    status TEXT NOT NULL CHECK (status IN ('unknown', 'overlap', 'possible_gap')),
    created_at INTEGER NOT NULL,
    checked_at INTEGER NOT NULL,
    PRIMARY KEY (previous_capture_id, current_capture_id),
    CHECK (previous_capture_id != current_capture_id)
);

CREATE INDEX feed_continuity_checks_current ON feed_continuity_checks(current_capture_id);
