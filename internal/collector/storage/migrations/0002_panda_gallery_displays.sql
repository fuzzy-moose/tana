-- Each record of a displayed Panda gallery keeps its own time. The seen state is
-- the user's history, not a projection, so it has no foreign key and survives
-- every change of catalog_galleries (ADR 0019). The key leads with gallery_id,
-- so the displayed flag is an index lookup.
CREATE TABLE panda_gallery_displays (
    gallery_id BIGINT NOT NULL CHECK (gallery_id > 0),
    displayed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (gallery_id, displayed_at)
);
