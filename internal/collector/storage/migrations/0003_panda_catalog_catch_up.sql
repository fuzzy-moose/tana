-- The one catch-up position: every gallery at or before it in catalog order
-- counts as seen. It stores position values, not a row reference, so it has no
-- foreign key and also covers galleries that join the catalog later (ADR 0019).
-- No row means no catch-up.
CREATE TABLE panda_catalog_catch_up (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    ordered_at TIMESTAMPTZ NOT NULL,
    gallery_id BIGINT NOT NULL,
    caught_up_at TIMESTAMPTZ NOT NULL
);
