-- Whether a display is evidence for the Panda score model. A display recorded
-- while a listing showed only the galleries the score filter hides is not, nor
-- is one from before verdicts were possible: that time ends when migration 4
-- created panda_gallery_verdicts, so the cutoff is its schema_migrations
-- applied_at. A database that gets migrations 4 and 6 in one transaction shares
-- that time, so every existing display is marked. Every display still counts
-- for the seen state.
ALTER TABLE panda_gallery_displays ADD COLUMN evidence BOOLEAN NOT NULL DEFAULT true;
UPDATE panda_gallery_displays SET evidence = false
WHERE displayed_at < (SELECT applied_at FROM schema_migrations WHERE version = 4);
