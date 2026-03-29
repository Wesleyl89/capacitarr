-- +goose Up
-- 3.0: Per-disk-group execution modes, sunset queue, and sunset preferences.
-- Breaking change: execution mode moves from global (preference_sets) to
-- per-disk-group (disk_groups.mode). All existing groups start in dry-run.
-- The global execution_mode column rename is handled by a Go fixup in migrate.go
-- because SQLite column renames require conditional logic.

-- Add per-disk-group mode (all existing groups start in dry-run)
ALTER TABLE disk_groups ADD COLUMN mode TEXT NOT NULL DEFAULT 'dry-run';
ALTER TABLE disk_groups ADD COLUMN sunset_pct REAL DEFAULT NULL;

-- Create sunset_queue table
CREATE TABLE IF NOT EXISTS sunset_queue (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    media_name            TEXT NOT NULL,
    media_type            TEXT NOT NULL,
    tmdb_id               INTEGER,
    integration_id        INTEGER REFERENCES integration_configs(id),
    external_id           TEXT,
    size_bytes            INTEGER NOT NULL DEFAULT 0,
    score                 REAL NOT NULL DEFAULT 0,
    score_details         TEXT,
    poster_url            TEXT,
    disk_group_id         INTEGER NOT NULL REFERENCES disk_groups(id),
    collection_group      TEXT,
    trigger               TEXT NOT NULL DEFAULT 'engine',
    deletion_date         DATE NOT NULL,
    label_applied         INTEGER NOT NULL DEFAULT 0,
    poster_overlay_active INTEGER NOT NULL DEFAULT 0,
    created_at            DATETIME,
    updated_at            DATETIME
);

CREATE INDEX IF NOT EXISTS idx_sunset_queue_disk_group ON sunset_queue(disk_group_id);
CREATE INDEX IF NOT EXISTS idx_sunset_queue_tmdb_id ON sunset_queue(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_sunset_queue_media_name ON sunset_queue(media_name);
CREATE INDEX IF NOT EXISTS idx_sunset_queue_deletion_date ON sunset_queue(deletion_date);

-- Add sunset preferences to preference_sets
ALTER TABLE preference_sets ADD COLUMN sunset_days INTEGER NOT NULL DEFAULT 30;
ALTER TABLE preference_sets ADD COLUMN sunset_label TEXT NOT NULL DEFAULT 'capacitarr-sunset';
ALTER TABLE preference_sets ADD COLUMN poster_overlay_enabled INTEGER NOT NULL DEFAULT 0;

-- Add sunset notification toggle to notification_configs
ALTER TABLE notification_configs ADD COLUMN on_sunset_activity INTEGER NOT NULL DEFAULT 1;

-- Update schema lineage marker
UPDATE schema_info SET value = 'v3' WHERE key = 'schema_family';

-- +goose Down
-- Note: ALTER TABLE DROP COLUMN requires SQLite 3.35.0+ (2021-03-12).
-- The Capacitarr Docker image uses a modern SQLite version via ncruces/go-sqlite3,
-- so DROP COLUMN is supported. If running on an older SQLite, the down migration
-- will fail on the DROP COLUMN statements — manual table rebuild would be needed.
ALTER TABLE disk_groups DROP COLUMN mode;
ALTER TABLE disk_groups DROP COLUMN sunset_pct;
ALTER TABLE preference_sets DROP COLUMN sunset_days;
ALTER TABLE preference_sets DROP COLUMN sunset_label;
ALTER TABLE preference_sets DROP COLUMN poster_overlay_enabled;
ALTER TABLE notification_configs DROP COLUMN on_sunset_activity;
DROP INDEX IF EXISTS idx_sunset_queue_deletion_date;
DROP INDEX IF EXISTS idx_sunset_queue_media_name;
DROP INDEX IF EXISTS idx_sunset_queue_tmdb_id;
DROP INDEX IF EXISTS idx_sunset_queue_disk_group;
DROP TABLE IF EXISTS sunset_queue;
UPDATE schema_info SET value = 'v2' WHERE key = 'schema_family';
