-- A display only makes a gallery seen; the Panda score model learns from the
-- Panda favorites alone, so whether a display was evidence has no use.
ALTER TABLE panda_gallery_displays DROP COLUMN evidence;
