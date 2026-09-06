CREATE TABLE panda_enrichments (
    gallery_id INTEGER PRIMARY KEY REFERENCES galleries(id) ON DELETE CASCADE,
    panda_id INTEGER NOT NULL CHECK (panda_id > 0),
    next_attempt_at INTEGER NOT NULL DEFAULT 0,
    failures INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX panda_enrichments_due ON panda_enrichments(next_attempt_at, gallery_id);
