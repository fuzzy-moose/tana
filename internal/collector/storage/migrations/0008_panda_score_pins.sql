-- The user's pinned weights: a number that replaces the score model's weight
-- of one tag, uploader or Panda gallery category in the Panda gallery score.
-- A tag pin references catalog_tags, whose rows the projection only inserts
-- and never deletes or renumbers, so the reference survives every projection
-- change. Like an override, a pin is the user's judgment and belongs to no
-- score model, so it references no model and survives every calculation and
-- every change of the active model; it references no gallery row either
-- (ADR 0019). An uploader or category pin keeps its value as text. The two
-- unique indexes serve the pin probe of the score expression.
CREATE TABLE panda_score_pins (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('tag', 'uploader', 'category')),
    tag_id BIGINT REFERENCES catalog_tags (id),
    value TEXT NOT NULL DEFAULT '',
    weight DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (CASE WHEN kind = 'tag' THEN tag_id IS NOT NULL AND value = ''
        ELSE tag_id IS NULL AND value <> '' END)
);
CREATE UNIQUE INDEX panda_score_pin_tag ON panda_score_pins (tag_id) WHERE kind = 'tag';
CREATE UNIQUE INDEX panda_score_pin_value ON panda_score_pins (kind, value) WHERE kind <> 'tag';
