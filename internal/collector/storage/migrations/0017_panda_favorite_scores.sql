-- The held-out favorite scores of the active score model in tenths, lowest
-- first, that the score grades of the catalog listing compare with. Applying
-- a model, changing a pin, an override or the author bonus, and a collector
-- start recalculate them; favorite syncs and changes to the tags of favorites
-- wait for the next of these, like the model's weights. model_id names
-- the model they belong to; scores of any other model are stale. A separate
-- row, so that these writes take no lock on panda_score_models. The table
-- holds exactly one row.
CREATE TABLE panda_favorite_scores (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    model_id BIGINT,
    scores INTEGER[] NOT NULL DEFAULT '{}'
);
INSERT INTO panda_favorite_scores (id) VALUES (1);
