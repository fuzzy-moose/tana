CREATE TABLE panda_downloads (
    gallery_id INTEGER PRIMARY KEY CHECK (gallery_id > 0),
    token TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'deleting')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    retry_at INTEGER NOT NULL DEFAULT 0,
    failures INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX panda_downloads_queue ON panda_downloads(state, created_at, gallery_id);
