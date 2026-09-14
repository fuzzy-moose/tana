ALTER TABLE catalog_galleries ADD COLUMN category TEXT NOT NULL DEFAULT '';

UPDATE catalog_galleries
SET category = COALESCE((
    SELECT CASE WHEN lower(trim(json_extract(body, '$.category'))) IN
        ('doujinshi', 'manga', 'artist cg', 'game cg', 'western', 'non-h', 'misc', 'cosplay', 'asian porn', 'image set')
        THEN lower(trim(json_extract(body, '$.category'))) ELSE '' END
    FROM gallery_metadata WHERE gallery_id = catalog_galleries.gallery_id
), '');

CREATE INDEX catalog_category_posted ON catalog_galleries(category, posted DESC, gallery_id DESC);
CREATE INDEX favorites_gallery_added ON favorites(gallery_id, added_at DESC);
