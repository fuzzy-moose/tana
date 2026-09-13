-- The shared deadline does not identify its originating request. Carry it
-- forward only to the main unauthenticated group; the authenticated group
-- intentionally starts without an inherited ban. Per-job retry deadlines stay
-- untouched, so already waiting work may remain delayed by the former cooldown.
CREATE TABLE panda_ban_scoped (
    id INTEGER PRIMARY KEY CHECK (id IN (1, 2)),
    until_at INTEGER NOT NULL DEFAULT 0
);

INSERT INTO panda_ban_scoped (id, until_at) SELECT id, until_at FROM panda_ban;
INSERT INTO panda_ban_scoped (id) VALUES (2);

DROP TABLE panda_ban;
ALTER TABLE panda_ban_scoped RENAME TO panda_ban;
