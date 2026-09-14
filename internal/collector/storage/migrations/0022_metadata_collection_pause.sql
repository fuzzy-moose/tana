CREATE TABLE metadata_collection_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    main_background_paused INTEGER NOT NULL DEFAULT 0 CHECK (main_background_paused IN (0, 1))
);

INSERT INTO metadata_collection_settings (id) VALUES (1);
