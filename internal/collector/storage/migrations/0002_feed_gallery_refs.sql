CREATE TABLE feed_gallery_refs (
    raw_feed_id INTEGER NOT NULL REFERENCES raw_feeds(id),
    gallery_id INTEGER NOT NULL REFERENCES gallery_refs(gallery_id),
    PRIMARY KEY (raw_feed_id, gallery_id)
);

CREATE INDEX feed_gallery_refs_gallery ON feed_gallery_refs(gallery_id);
