-- The Panda score model learns only from the Panda favorites, so verdicts have
-- no use and go without an export. Their display records stay, so a gallery
-- that had a verdict stays seen.
DROP TABLE panda_gallery_verdicts;
