CREATE TABLE catalog_backfill (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    completed INTEGER NOT NULL DEFAULT 0 CHECK (completed IN (0, 1))
);

INSERT INTO catalog_backfill (id) VALUES (1);
