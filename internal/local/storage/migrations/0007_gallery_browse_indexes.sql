CREATE INDEX galleries_title ON galleries(title COLLATE NOCASE, id);

DROP INDEX gallery_tags_tag;
CREATE INDEX gallery_tags_tag ON gallery_tags(tag_id, gallery_id);
