ALTER TABLE favorites ADD COLUMN committed_added_at INTEGER;
UPDATE favorites SET committed_added_at = added_at;

CREATE TABLE favorite_syncs (
    category_id INTEGER PRIMARY KEY REFERENCES favorite_categories(id),
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'success', 'failed')),
    full INTEGER NOT NULL,
    followup_full INTEGER NOT NULL DEFAULT 0,
    queued_at INTEGER NOT NULL,
    next_url TEXT NOT NULL DEFAULT '',
    pages_saved INTEGER NOT NULL DEFAULT 0,
    entries_saved INTEGER NOT NULL DEFAULT 0,
    last_saved_at INTEGER NOT NULL DEFAULT 0,
    last_added_at INTEGER NOT NULL DEFAULT 0,
    restarted INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    last_error_at INTEGER NOT NULL DEFAULT 0,
    retry_at INTEGER NOT NULL DEFAULT 0,
    failures INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE favorite_sync_seen (
    category_id INTEGER NOT NULL REFERENCES favorite_syncs(category_id),
    gallery_id INTEGER NOT NULL,
    PRIMARY KEY (category_id, gallery_id)
);

CREATE TABLE favorite_sync_pages (
    category_id INTEGER NOT NULL REFERENCES favorite_syncs(category_id),
    url TEXT NOT NULL,
    PRIMARY KEY (category_id, url)
);
