-- The user's score overrides: "always hide" or "never hide" for one tag,
-- uploader or Panda gallery category. A tag override references catalog_tags,
-- whose rows the projection only inserts and never deletes or renumbers, so the
-- reference survives every projection change. Like a verdict, an override is
-- the user's judgment, not a projection, so it references no gallery row
-- (ADR 0019). An uploader or category override keeps its value as text.
CREATE TABLE panda_score_overrides (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('tag', 'uploader', 'category')),
    tag_id BIGINT REFERENCES catalog_tags (id),
    value TEXT NOT NULL DEFAULT '',
    rule TEXT NOT NULL CHECK (rule IN ('always_hide', 'never_hide')),
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (CASE WHEN kind = 'tag' THEN tag_id IS NOT NULL AND value = ''
        ELSE tag_id IS NULL AND value <> '' END)
);
CREATE UNIQUE INDEX panda_score_override_tag ON panda_score_overrides (tag_id) WHERE kind = 'tag';
CREATE UNIQUE INDEX panda_score_override_value ON panda_score_overrides (kind, value) WHERE kind <> 'tag';
