CREATE TABLE favorite_download_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    baseline_state TEXT NOT NULL DEFAULT 'not_started' CHECK (baseline_state IN ('not_started', 'collecting', 'ready'))
);
INSERT INTO favorite_download_settings (id) VALUES (1);

CREATE TABLE favorite_download_categories (
    category INTEGER PRIMARY KEY CHECK (category BETWEEN 0 AND 9)
);

CREATE TABLE favorite_download_baseline (
    category INTEGER PRIMARY KEY CHECK (category BETWEEN 0 AND 9)
);

CREATE TABLE favorite_observations (
    gallery_id INTEGER PRIMARY KEY REFERENCES gallery_refs(gallery_id)
);
INSERT INTO favorite_observations (gallery_id) SELECT DISTINCT gallery_id FROM favorites;
