-- The user's settings of the Panda gallery score that belong to no score
-- model: the author bonus, the one weight that the tag of every favorite
-- author weighs at least while a model is active, unless the tag has a pinned
-- weight. NULL means the bonus is off. Like a pin, the bonus is the user's
-- judgment, so it survives every calculation and every change of the active
-- model (ADR 0019). The table holds exactly one row.
CREATE TABLE panda_score_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    author_bonus DOUBLE PRECISION CHECK (author_bonus > 0)
);
INSERT INTO panda_score_settings (id) VALUES (1);
