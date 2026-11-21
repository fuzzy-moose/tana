-- The default filter of the scope counts when the model was applied: the
-- scope its counts come from, so that a later preview with another default
-- filter does not change which favorites its scores hold. query is NULL
-- without a scope.
ALTER TABLE panda_favorite_scores
    ADD COLUMN query TEXT,
    ADD COLUMN categories TEXT[] NOT NULL DEFAULT '{}';
