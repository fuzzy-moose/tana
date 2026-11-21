-- A score model always learns from the Panda favorites, so a model stores no
-- setting for them.
ALTER TABLE panda_score_models DROP COLUMN favorites;
