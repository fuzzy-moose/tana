CREATE TABLE sitemap_run (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    state TEXT NOT NULL CHECK (state IN ('idle', 'running', 'completed', 'incomplete', 'cancelled')),
    force INTEGER NOT NULL DEFAULT 0,
    index_url TEXT NOT NULL DEFAULT '',
    index_ready INTEGER NOT NULL DEFAULT 0,
    index_failures INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    retry_at INTEGER NOT NULL DEFAULT 0,
    next_request_at INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
INSERT INTO sitemap_run (id, state) VALUES (1, 'idle');

CREATE TABLE sitemap_children (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'running', 'completed', 'skipped', 'failed')),
    failures INTEGER NOT NULL DEFAULT 0,
    retry_at INTEGER NOT NULL DEFAULT 0,
    references_found INTEGER NOT NULL DEFAULT 0,
    references_imported INTEGER NOT NULL DEFAULT 0,
    invalid_locations INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sitemap_children_pending ON sitemap_children(state, retry_at, id);

-- Validators certify successful imports, independently of the current run.
CREATE TABLE sitemap_validators (
    url TEXT PRIMARY KEY,
    etag TEXT NOT NULL,
    last_modified TEXT NOT NULL,
    response_url TEXT NOT NULL
);
