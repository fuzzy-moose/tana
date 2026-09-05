CREATE TABLE sources (
    id TEXT PRIMARY KEY NOT NULL,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path TEXT NOT NULL CHECK (length(path) > 0),
    kind TEXT NOT NULL CHECK (kind IN ('directory', 'archive')),
    UNIQUE (library_id, path)
);

CREATE TABLE source_files (
    id TEXT PRIMARY KEY NOT NULL,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    path TEXT NOT NULL CHECK (length(path) > 0),
    UNIQUE (source_id, path)
);

CREATE TABLE galleries (
    id TEXT PRIMARY KEY NOT NULL,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0)
);

CREATE TABLE gallery_pages (
    gallery_id TEXT NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    source_file_id TEXT NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
    PRIMARY KEY (gallery_id, position)
);

CREATE INDEX gallery_pages_source_file ON gallery_pages(source_file_id);

-- Position preserves order; page numbers are its consecutive ranks. Cascading
-- deletions can leave position gaps without changing a gallery's identity or
-- requiring library/source repositories to rewrite its surviving pages.
