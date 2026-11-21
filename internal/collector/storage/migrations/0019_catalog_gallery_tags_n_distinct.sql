-- ANALYZE samples too few rows of this table to see how many galleries it
-- holds; it estimated one per 200 assignments where a gallery has about 13
-- tags, so the planner overrated every per-gallery tag probe by as much. A
-- negative value is a share of the rows, so it holds as the catalog grows.
ALTER TABLE catalog_gallery_tags ALTER COLUMN gallery_id SET (n_distinct = -0.075);
ANALYZE catalog_gallery_tags (gallery_id);
