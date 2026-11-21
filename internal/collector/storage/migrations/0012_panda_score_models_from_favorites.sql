-- A score model learns only from the Panda favorites inside the default
-- filter, against the Panda catalog under the same filter. Models calculated
-- from verdicts are deleted, so the score filter stays off until a new model
-- is applied; their weights follow by the foreign key. Each weight keeps the
-- numbers of favorites and catalog galleries in scope that carry its value,
-- and each model the numbers of favorites and catalog galleries in scope it
-- was calculated from. The tables are empty when the columns are added, so
-- they need no default.
DELETE FROM panda_score_models;
ALTER TABLE panda_score_weights
    DROP COLUMN interesting,
    DROP COLUMN not_for_me,
    DROP COLUMN neutral,
    ADD COLUMN favorites INTEGER NOT NULL,
    ADD COLUMN catalog INTEGER NOT NULL;
ALTER TABLE panda_score_models
    ADD COLUMN favorites_in_scope INTEGER NOT NULL,
    ADD COLUMN catalog_in_scope INTEGER NOT NULL;
