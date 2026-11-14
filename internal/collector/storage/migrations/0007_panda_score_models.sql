-- Panda score models: at most one active model with its threshold in tenths,
-- and the candidates that previews calculate, which only the active row
-- outlives. Each weight belongs to one model and names its tag, uploader or
-- category like an override does: a tag by its catalog_tags row, the others by
-- their text. The two unique indexes serve the score lookup for one gallery.
CREATE TABLE panda_score_models (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    active BOOLEAN NOT NULL DEFAULT false,
    favorites BOOLEAN NOT NULL,
    calculated_at TIMESTAMPTZ NOT NULL,
    threshold_tenths INTEGER,
    CHECK (active = (threshold_tenths IS NOT NULL))
);
CREATE UNIQUE INDEX panda_score_model_active ON panda_score_models (active) WHERE active;

CREATE TABLE panda_score_weights (
    model_id BIGINT NOT NULL REFERENCES panda_score_models (id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('tag', 'uploader', 'category')),
    tag_id BIGINT REFERENCES catalog_tags (id),
    value TEXT NOT NULL DEFAULT '',
    weight DOUBLE PRECISION NOT NULL,
    interesting INTEGER NOT NULL,
    not_for_me INTEGER NOT NULL,
    neutral INTEGER NOT NULL,
    CHECK (CASE WHEN kind = 'tag' THEN tag_id IS NOT NULL AND value = ''
        ELSE tag_id IS NULL AND value <> '' END)
);
CREATE UNIQUE INDEX panda_score_weight_tag ON panda_score_weights (model_id, tag_id) WHERE kind = 'tag';
CREATE UNIQUE INDEX panda_score_weight_value ON panda_score_weights (model_id, kind, value) WHERE kind <> 'tag';
