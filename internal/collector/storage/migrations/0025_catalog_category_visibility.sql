-- Visibility trails the ordering keys to preserve upload order when expunged galleries are included.
DROP INDEX catalog_category_posted;
CREATE INDEX catalog_category_posted ON catalog_galleries(category, posted DESC, gallery_id DESC, expunged);
