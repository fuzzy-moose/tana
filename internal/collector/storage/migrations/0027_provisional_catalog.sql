ALTER TABLE gallery_refs ADD COLUMN feed_metadata_base_at INTEGER;

-- Historical feed bodies are gone; their first captured sighting remains known.
UPDATE gallery_refs SET feed_metadata_base_at = (
    SELECT MIN(f.captured_at) FROM feed_gallery_refs s
    JOIN raw_feeds f ON f.id = s.raw_feed_id WHERE s.gallery_id = gallery_refs.gallery_id
) WHERE EXISTS (SELECT 1 FROM feed_gallery_refs s WHERE s.gallery_id = gallery_refs.gallery_id);

CREATE TABLE catalog_galleries_next (
    gallery_id INTEGER PRIMARY KEY REFERENCES gallery_refs(gallery_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    title_lower TEXT NOT NULL,
    title_japanese_lower TEXT NOT NULL,
    thumbnail_url TEXT NOT NULL,
    page_count INTEGER,
    posted INTEGER,
    ordered_at INTEGER NOT NULL,
    expunged INTEGER CHECK (expunged IN (0, 1)),
    category TEXT NOT NULL DEFAULT '',
    metadata_source TEXT NOT NULL DEFAULT 'collected' CHECK (metadata_source IN ('feed', 'collected')),
    first_seen_at INTEGER,
    feed_captured_at INTEGER,
    feed_capture_id INTEGER,
    uploader TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT ''
);

INSERT INTO catalog_galleries_next
    (gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, ordered_at, expunged, category)
SELECT gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, posted, expunged, category
FROM catalog_galleries;

CREATE TABLE catalog_gallery_tags_next (
    gallery_id INTEGER NOT NULL REFERENCES catalog_galleries_next(gallery_id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES catalog_tags(id),
    PRIMARY KEY (gallery_id, tag_id)
);
INSERT INTO catalog_gallery_tags_next SELECT gallery_id, tag_id FROM catalog_gallery_tags;
DROP TABLE catalog_gallery_tags;
DROP TABLE catalog_galleries;
ALTER TABLE catalog_galleries_next RENAME TO catalog_galleries;
ALTER TABLE catalog_gallery_tags_next RENAME TO catalog_gallery_tags;

CREATE INDEX catalog_posted ON catalog_galleries(ordered_at DESC, gallery_id DESC);
CREATE INDEX catalog_visible_posted ON catalog_galleries(ordered_at DESC, gallery_id DESC) WHERE expunged IS NOT 1;
CREATE INDEX catalog_category_posted ON catalog_galleries(category, ordered_at DESC, gallery_id DESC, expunged);
CREATE INDEX catalog_tag_galleries ON catalog_gallery_tags(tag_id, gallery_id);

CREATE TRIGGER catalog_collected_metadata_deleted AFTER DELETE ON gallery_metadata BEGIN
    DELETE FROM catalog_galleries WHERE gallery_id = OLD.gallery_id AND metadata_source = 'collected';
END;
