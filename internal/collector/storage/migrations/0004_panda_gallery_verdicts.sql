-- One verdict per Panda gallery, with the time it was given. A verdict is the
-- user's judgment, not a projection, so it has no foreign key and survives
-- every change of catalog_galleries (ADR 0019). No secondary index: the table
-- holds the few galleries the user judged, and the verdict views read it whole.
CREATE TABLE panda_gallery_verdicts (
    gallery_id BIGINT PRIMARY KEY CHECK (gallery_id > 0),
    verdict TEXT NOT NULL CHECK (verdict IN ('interesting', 'not_for_me')),
    given_at TIMESTAMPTZ NOT NULL
);
