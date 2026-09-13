CREATE TABLE panda_ban_scoped (
    id INTEGER PRIMARY KEY CHECK (id IN (1, 2)),
    until_at INTEGER NOT NULL DEFAULT 0
);

INSERT INTO panda_ban_scoped (id, until_at) SELECT id, until_at FROM panda_ban;
INSERT INTO panda_ban_scoped (id) VALUES (2);

DROP TABLE panda_ban;
ALTER TABLE panda_ban_scoped RENAME TO panda_ban;
