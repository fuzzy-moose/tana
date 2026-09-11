CREATE TABLE reference_imports (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    filename TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('processing', 'validating', 'completed', 'cancelled')),
    created_at INTEGER NOT NULL,
    completed_at INTEGER,
    has_file INTEGER NOT NULL DEFAULT 1,
    size_bytes INTEGER NOT NULL,
    processed_bytes INTEGER NOT NULL DEFAULT 0,
    reference_count INTEGER NOT NULL DEFAULT 0,
    duplicates INTEGER NOT NULL DEFAULT 0,
    invalid INTEGER NOT NULL DEFAULT 0,
    known INTEGER NOT NULL DEFAULT 0,
    imported INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    pending INTEGER NOT NULL DEFAULT 0,
    cancelled INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX reference_imports_completion ON reference_imports(completed_at);
CREATE INDEX reference_imports_active ON reference_imports(sequence)
WHERE status IN ('processing', 'validating');

-- Outcomes belong to each import. Completing or retrying one owner must not
-- rewrite another owner's terminal outcome for the same reference.
CREATE TABLE reference_import_entries (
    id INTEGER PRIMARY KEY,
    import_id TEXT NOT NULL REFERENCES reference_imports(id) ON DELETE CASCADE,
    gallery_id INTEGER NOT NULL,
    token TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'known', 'imported', 'failed', 'cancelled')),
    UNIQUE (import_id, gallery_id, token)
);
CREATE INDEX reference_import_entries_pending_ref ON reference_import_entries(gallery_id, token)
WHERE status = 'pending';
CREATE INDEX reference_import_entries_pending_order ON reference_import_entries(import_id, id)
WHERE status = 'pending';
CREATE INDEX reference_import_entries_outcomes ON reference_import_entries(import_id, status);

-- Summary reads stay constant-sized even for imports with hundreds of thousands
-- of entries; counters commit in the same transaction as their outcomes.
CREATE TRIGGER reference_import_entry_insert AFTER INSERT ON reference_import_entries BEGIN
    UPDATE reference_imports SET
        reference_count = reference_count + 1,
        known = known + (NEW.status = 'known'),
        imported = imported + (NEW.status = 'imported'),
        failed = failed + (NEW.status = 'failed'),
        pending = pending + (NEW.status = 'pending'),
        cancelled = cancelled + (NEW.status = 'cancelled')
    WHERE id = NEW.import_id;
END;
CREATE TRIGGER reference_import_entry_outcome AFTER UPDATE OF status ON reference_import_entries
WHEN OLD.status != NEW.status BEGIN
    UPDATE reference_imports SET
        known = known + (NEW.status = 'known') - (OLD.status = 'known'),
        imported = imported + (NEW.status = 'imported') - (OLD.status = 'imported'),
        failed = failed + (NEW.status = 'failed') - (OLD.status = 'failed'),
        pending = pending + (NEW.status = 'pending') - (OLD.status = 'pending'),
        cancelled = cancelled + (NEW.status = 'cancelled') - (OLD.status = 'cancelled')
    WHERE id = NEW.import_id;
END;
