CREATE TABLE panda_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    state TEXT NOT NULL CHECK (state IN ('running', 'paused', 'stopped', 'completed', 'completed_with_errors')),
    data TEXT NOT NULL
);

CREATE UNIQUE INDEX panda_deliveries_active ON panda_deliveries ((1))
    WHERE state IN ('running', 'paused');
