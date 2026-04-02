-- +goose Up
-- Remove unused 'album' media type from approval_queue CHECK constraint.
-- Lidarr integration produces 'artist'-level items, not album-level.
-- SQLite requires table recreation to alter CHECK constraints.
CREATE TABLE approval_queue_new (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    media_name       TEXT    NOT NULL,
    media_type       TEXT    NOT NULL CHECK(media_type IN ('movie','show','season','episode','artist','book')),
    score_details    TEXT,
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    score            REAL    NOT NULL DEFAULT 0,
    poster_url       TEXT    NOT NULL DEFAULT '',
    integration_id   INTEGER NOT NULL REFERENCES integration_configs(id) ON DELETE CASCADE,
    external_id      TEXT    NOT NULL DEFAULT '',
    disk_group_id    INTEGER REFERENCES disk_groups(id) ON DELETE SET NULL,
    status           TEXT    NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
    "trigger"        TEXT    NOT NULL DEFAULT 'engine',
    user_initiated   INTEGER NOT NULL DEFAULT 0,
    collection_group TEXT    NOT NULL DEFAULT '',
    snoozed_until    DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO approval_queue_new SELECT * FROM approval_queue;
DROP TABLE approval_queue;
ALTER TABLE approval_queue_new RENAME TO approval_queue;
CREATE INDEX IF NOT EXISTS idx_approval_queue_status ON approval_queue(status);
CREATE INDEX IF NOT EXISTS idx_approval_queue_disk_group ON approval_queue(disk_group_id);

-- +goose Down
-- Re-add 'album' to the CHECK constraint
CREATE TABLE approval_queue_old (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    media_name       TEXT    NOT NULL,
    media_type       TEXT    NOT NULL CHECK(media_type IN ('movie','show','season','episode','artist','album','book')),
    score_details    TEXT,
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    score            REAL    NOT NULL DEFAULT 0,
    poster_url       TEXT    NOT NULL DEFAULT '',
    integration_id   INTEGER NOT NULL REFERENCES integration_configs(id) ON DELETE CASCADE,
    external_id      TEXT    NOT NULL DEFAULT '',
    disk_group_id    INTEGER REFERENCES disk_groups(id) ON DELETE SET NULL,
    status           TEXT    NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
    "trigger"        TEXT    NOT NULL DEFAULT 'engine',
    user_initiated   INTEGER NOT NULL DEFAULT 0,
    collection_group TEXT    NOT NULL DEFAULT '',
    snoozed_until    DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO approval_queue_old SELECT * FROM approval_queue;
DROP TABLE approval_queue;
ALTER TABLE approval_queue_old RENAME TO approval_queue;
CREATE INDEX IF NOT EXISTS idx_approval_queue_status ON approval_queue(status);
CREATE INDEX IF NOT EXISTS idx_approval_queue_disk_group ON approval_queue(disk_group_id);
