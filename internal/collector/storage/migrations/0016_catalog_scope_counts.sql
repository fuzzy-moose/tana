-- The catalog in scope of the default filter, kept as a projection so that a
-- score model calculation reads its counts instead of counting the catalog.
-- query is NULL until a preview names a default filter. While ready is false,
-- the galleries up to built_through are counted.
CREATE TABLE catalog_scope (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    query TEXT,
    categories TEXT[] NOT NULL DEFAULT '{}',
    built_through BIGINT NOT NULL DEFAULT 0,
    ready BOOLEAN NOT NULL DEFAULT false,
    galleries INTEGER NOT NULL DEFAULT 0
);
INSERT INTO catalog_scope (id) VALUES (1);

-- Each counted gallery with the values it was counted with, so that a change
-- takes back exactly what the gallery added.
CREATE TABLE catalog_scope_galleries (
    gallery_id BIGINT PRIMARY KEY,
    uploader TEXT NOT NULL,
    category TEXT NOT NULL,
    tag_ids BIGINT[] NOT NULL
);

-- tag_id is 0 and value '' where the kind does not use them.
CREATE TABLE catalog_scope_counts (
    kind TEXT NOT NULL,
    tag_id BIGINT NOT NULL,
    value TEXT NOT NULL,
    galleries INTEGER NOT NULL,
    PRIMARY KEY (kind, tag_id, value)
);

-- Galleries changed since they were last counted. Every change goes through
-- these triggers, cascades included. A change updates a waiting row instead of
-- skipping it, so that it waits for a worker that holds the row and is queued
-- again after that worker removes it.
CREATE TABLE catalog_scope_changes (
    gallery_id BIGINT PRIMARY KEY
);

CREATE FUNCTION catalog_scope_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO catalog_scope_changes (gallery_id)
    VALUES (CASE WHEN TG_OP = 'DELETE' THEN OLD.gallery_id ELSE NEW.gallery_id END)
    ON CONFLICT (gallery_id) DO UPDATE SET gallery_id = excluded.gallery_id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER catalog_galleries_scope_changed
AFTER INSERT OR DELETE OR UPDATE OF title_lower, title_japanese_lower, category, uploader ON catalog_galleries
FOR EACH ROW EXECUTE FUNCTION catalog_scope_changed();

CREATE TRIGGER catalog_gallery_tags_scope_changed
AFTER INSERT OR DELETE ON catalog_gallery_tags
FOR EACH ROW EXECUTE FUNCTION catalog_scope_changed();
