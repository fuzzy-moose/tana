-- The favorite authors: each artist tag that at least two Panda favorites
-- carry in catalog_gallery_tags, with the number of those favorites. A gallery
-- in several favorite collections counts once, and a favorite without catalog
-- tags counts for no author. The score expression probes this table by key
-- instead of counting favorites per candidate. Only the transaction that
-- completes a Panda favorite sync refreshes it, and only a completed full
-- re-sync removes an author (ADR 0019). This migration fills it once from the
-- stored favorites.
CREATE TABLE favorite_authors (
    tag_id BIGINT PRIMARY KEY REFERENCES catalog_tags (id),
    favorites INTEGER NOT NULL
);

INSERT INTO favorite_authors (tag_id, favorites)
SELECT t.id, count(DISTINCT f.gallery_id)
FROM favorites f
JOIN catalog_gallery_tags gt ON gt.gallery_id = f.gallery_id
JOIN catalog_tags t ON t.id = gt.tag_id
WHERE t.namespace = 'artist'
GROUP BY t.id
HAVING count(DISTINCT f.gallery_id) >= 2;
