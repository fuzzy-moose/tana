CREATE TABLE libraries (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    path TEXT UNIQUE NOT NULL,
    availability TEXT NOT NULL DEFAULT 'unknown'
        CHECK (availability IN ('unknown', 'available', 'unavailable')),
    last_checked_at INTEGER
);

-- Future library-owned tables must reference libraries(id) ON DELETE CASCADE.
