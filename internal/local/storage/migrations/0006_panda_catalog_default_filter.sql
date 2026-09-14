CREATE TABLE panda_catalog_default_filter (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    query TEXT NOT NULL,
    categories TEXT NOT NULL
);

INSERT INTO panda_catalog_default_filter (id, query, categories) VALUES (1, '', '[]');
