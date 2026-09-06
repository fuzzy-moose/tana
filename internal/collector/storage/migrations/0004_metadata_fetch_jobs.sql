CREATE TABLE metadata_fetch_jobs (
    id TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);
CREATE INDEX metadata_fetch_jobs_completion ON metadata_fetch_jobs(completed_at);

-- Pending requests deliberately have no foreign key to gallery_refs: their
-- tokens have not necessarily been confirmed by Panda yet.
CREATE TABLE metadata_fetches (
    id INTEGER PRIMARY KEY,
    gallery_id INTEGER NOT NULL,
    token TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'successful', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    refreshed_at INTEGER
);
CREATE UNIQUE INDEX metadata_fetches_pending_ref ON metadata_fetches(gallery_id, token) WHERE status = 'pending';
CREATE INDEX metadata_fetches_pending_order ON metadata_fetches(id) WHERE status = 'pending';

CREATE TABLE metadata_fetch_job_entries (
    job_id TEXT NOT NULL REFERENCES metadata_fetch_jobs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    fetch_id INTEGER NOT NULL REFERENCES metadata_fetches(id),
    PRIMARY KEY (job_id, position)
);
CREATE INDEX metadata_fetch_job_entries_fetch ON metadata_fetch_job_entries(fetch_id);
