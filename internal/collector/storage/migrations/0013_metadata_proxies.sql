CREATE TABLE metadata_proxy_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1))
);
INSERT INTO metadata_proxy_settings (id) VALUES (1);

-- Endpoint history outlives channel configuration, including its credentials.
CREATE TABLE metadata_proxy_bans (
    endpoint TEXT PRIMARY KEY,
    until_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE metadata_proxy_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    endpoint TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL,
    password TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    revision INTEGER NOT NULL DEFAULT 1,
    deleting INTEGER NOT NULL DEFAULT 0 CHECK (deleting IN (0, 1)),
    ban_until INTEGER NOT NULL DEFAULT 0,
    failures INTEGER NOT NULL DEFAULT 0,
    retry_at INTEGER NOT NULL DEFAULT 0,
    auth_failed INTEGER NOT NULL DEFAULT 0 CHECK (auth_failed IN (0, 1)),
    last_error TEXT NOT NULL DEFAULT '',
    last_success_at INTEGER NOT NULL DEFAULT 0
);
