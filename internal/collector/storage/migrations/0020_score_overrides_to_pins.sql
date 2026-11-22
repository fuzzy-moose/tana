-- The score filter has pinned weights only: each score override becomes a pin
-- on the same tag, uploader or category, which replaces the weight of a pin
-- the value already has. "Always hide" becomes -100 and "never hide" +1000, so
-- that a gallery with both stays visible. The held-out favorite scores keep
-- their model and scope but lose their scores, which the refresh of the next
-- collector start calculates under the new pins.
UPDATE panda_score_pins p
SET weight = CASE o.rule WHEN 'always_hide' THEN -100 ELSE 1000 END
FROM panda_score_overrides o
WHERE p.kind = o.kind AND p.tag_id IS NOT DISTINCT FROM o.tag_id AND p.value = o.value;
INSERT INTO panda_score_pins (kind, tag_id, value, weight, created_at)
SELECT o.kind, o.tag_id, o.value, CASE o.rule WHEN 'always_hide' THEN -100 ELSE 1000 END, o.created_at
FROM panda_score_overrides o
WHERE NOT EXISTS (SELECT FROM panda_score_pins p
    WHERE p.kind = o.kind AND p.tag_id IS NOT DISTINCT FROM o.tag_id AND p.value = o.value);
UPDATE panda_favorite_scores SET scores = '{}' WHERE EXISTS (SELECT FROM panda_score_overrides);
DROP TABLE panda_score_overrides;
