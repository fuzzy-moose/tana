CREATE TABLE catalog_galleries (
    gallery_id INTEGER PRIMARY KEY REFERENCES gallery_metadata(gallery_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    title_lower TEXT NOT NULL,
    title_japanese_lower TEXT NOT NULL,
    thumbnail_url TEXT NOT NULL,
    page_count INTEGER NOT NULL,
    posted INTEGER NOT NULL,
    expunged INTEGER NOT NULL CHECK (expunged IN (0, 1))
);
CREATE INDEX catalog_posted ON catalog_galleries(posted DESC, gallery_id DESC);
CREATE INDEX catalog_visible_posted ON catalog_galleries(posted DESC, gallery_id DESC) WHERE expunged = 0;

CREATE TABLE catalog_tags (
    id INTEGER PRIMARY KEY,
    namespace TEXT NOT NULL,
    value TEXT NOT NULL,
    value_lower TEXT NOT NULL,
    UNIQUE (namespace, value_lower)
);
CREATE INDEX catalog_tag_value ON catalog_tags(value_lower, namespace);

CREATE TABLE catalog_gallery_tags (
    gallery_id INTEGER NOT NULL REFERENCES catalog_galleries(gallery_id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES catalog_tags(id),
    PRIMARY KEY (gallery_id, tag_id)
);
CREATE INDEX catalog_tag_galleries ON catalog_gallery_tags(tag_id, gallery_id);
