-- +goose Up
-- Baseline migration: captures the full schema as created by GORM AutoMigrate.
-- For existing databases this is a no-op (tables already exist).
-- For fresh installs this creates all tables from scratch.

CREATE TABLE IF NOT EXISTS auth_configs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    username   TEXT NOT NULL,
    password   TEXT NOT NULL,
    api_key    TEXT,
    created_at DATETIME,
    updated_at DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_configs_username ON auth_configs(username);
CREATE INDEX IF NOT EXISTS idx_auth_configs_api_key ON auth_configs(api_key);

CREATE TABLE IF NOT EXISTS library_histories (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp      DATETIME NOT NULL,
    total_capacity INTEGER  NOT NULL,
    used_capacity  INTEGER  NOT NULL,
    resolution     TEXT     NOT NULL,
    disk_group_id  INTEGER,
    created_at     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_library_histories_timestamp ON library_histories(timestamp);
CREATE INDEX IF NOT EXISTS idx_library_histories_resolution ON library_histories(resolution);
CREATE INDEX IF NOT EXISTS idx_library_histories_disk_group_id ON library_histories(disk_group_id);

CREATE TABLE IF NOT EXISTS integration_configs (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    type             TEXT    NOT NULL,
    name             TEXT    NOT NULL,
    url              TEXT    NOT NULL,
    api_key          TEXT    NOT NULL,
    enabled          NUMERIC DEFAULT 1,
    media_size_bytes INTEGER,
    media_count      INTEGER,
    last_sync        DATETIME,
    last_error       TEXT,
    created_at       DATETIME,
    updated_at       DATETIME
);
CREATE INDEX IF NOT EXISTS idx_integration_configs_type ON integration_configs(type);

CREATE TABLE IF NOT EXISTS disk_groups (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    mount_path    TEXT    NOT NULL,
    total_bytes   INTEGER NOT NULL,
    used_bytes    INTEGER NOT NULL,
    threshold_pct REAL    DEFAULT 85,
    target_pct    REAL    DEFAULT 75,
    created_at    DATETIME,
    updated_at    DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_disk_groups_mount_path ON disk_groups(mount_path);

CREATE TABLE IF NOT EXISTS preference_sets (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    log_level                TEXT NOT NULL DEFAULT 'info',
    audit_log_retention_days INTEGER NOT NULL DEFAULT 30,
    watch_history_weight     INTEGER DEFAULT 10,
    last_watched_weight      INTEGER DEFAULT 8,
    file_size_weight         INTEGER DEFAULT 6,
    rating_weight            INTEGER DEFAULT 5,
    time_in_library_weight   INTEGER DEFAULT 4,
    availability_weight      INTEGER DEFAULT 3,
    execution_mode           TEXT NOT NULL DEFAULT 'dry-run',
    tiebreaker_method        TEXT NOT NULL DEFAULT 'size_desc',
    updated_at               DATETIME
);

CREATE TABLE IF NOT EXISTS protection_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT NOT NULL,
    field      TEXT NOT NULL,
    operator   TEXT NOT NULL,
    value      TEXT NOT NULL,
    intensity  TEXT NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    media_name    TEXT NOT NULL,
    media_type    TEXT NOT NULL,
    reason        TEXT NOT NULL,
    score_details TEXT,
    action        TEXT NOT NULL,
    size_bytes    INTEGER,
    created_at    DATETIME
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_media_name ON audit_logs(media_name);

-- Seed default preferences if the table is empty (fresh install)
INSERT OR IGNORE INTO preference_sets (id, log_level, audit_log_retention_days,
    watch_history_weight, last_watched_weight, file_size_weight, rating_weight,
    time_in_library_weight, availability_weight, execution_mode, tiebreaker_method)
VALUES (1, 'info', 30, 10, 8, 6, 5, 4, 3, 'dry-run', 'size_desc');

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS protection_rules;
DROP TABLE IF EXISTS preference_sets;
DROP TABLE IF EXISTS disk_groups;
DROP TABLE IF EXISTS integration_configs;
DROP TABLE IF EXISTS library_histories;
DROP TABLE IF EXISTS auth_configs;
