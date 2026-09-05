CREATE TABLE libraries (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    path TEXT UNIQUE NOT NULL,
    availability TEXT NOT NULL DEFAULT 'unknown'
        CHECK (availability IN ('unknown', 'available', 'unavailable')),
    last_checked_at INTEGER
);

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
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    source_id TEXT REFERENCES sources(id) ON DELETE CASCADE
);

CREATE INDEX galleries_source ON galleries(source_id);

CREATE TABLE gallery_pages (
    gallery_id TEXT NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position > 0),
    source_file_id TEXT NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
    PRIMARY KEY (gallery_id, position)
);

CREATE INDEX gallery_pages_source_file ON gallery_pages(source_file_id);

-- Position preserves order; page numbers are its consecutive ranks. Cascading
-- deletions can leave gaps in independent galleries without renumbering writes.

CREATE TABLE namespaces (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT UNIQUE NOT NULL CHECK (
        length(name) > 0 AND name = trim(name) AND name NOT GLOB '*[^a-z]*'
    )
);

CREATE TABLE tags (
    id TEXT PRIMARY KEY NOT NULL,
    namespace_id TEXT NOT NULL REFERENCES namespaces(id),
    value TEXT NOT NULL CHECK (
        length(value) > 0 AND value = trim(value) AND value NOT GLOB '*[^a-z0-9 .-]*'
    ),
    UNIQUE (namespace_id, value)
);

CREATE TABLE gallery_tags (
    gallery_id TEXT NOT NULL REFERENCES galleries(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id),
    PRIMARY KEY (gallery_id, tag_id)
);

CREATE INDEX gallery_tags_tag ON gallery_tags(tag_id);
