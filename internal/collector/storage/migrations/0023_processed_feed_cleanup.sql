-- Capture timestamps and sightings still establish scheduling and continuity.
UPDATE raw_feeds SET body = X'' WHERE processed_at IS NOT NULL;
