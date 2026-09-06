CREATE TABLE favorite_categories (
    id INTEGER PRIMARY KEY,
    host TEXT NOT NULL,
    account_key TEXT NOT NULL,
    category INTEGER NOT NULL CHECK (category BETWEEN 0 AND 9),
    name TEXT NOT NULL,
    synced_at INTEGER NOT NULL,
    UNIQUE (host, account_key, category)
);

CREATE TABLE favorites (
    category_id INTEGER NOT NULL REFERENCES favorite_categories(id),
    gallery_id INTEGER NOT NULL REFERENCES gallery_refs(gallery_id),
    token TEXT NOT NULL,
    added_at INTEGER NOT NULL,
    PRIMARY KEY (category_id, gallery_id)
);

CREATE TABLE panda_ban (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    until_at INTEGER NOT NULL DEFAULT 0
);
INSERT INTO panda_ban (id) VALUES (1);
