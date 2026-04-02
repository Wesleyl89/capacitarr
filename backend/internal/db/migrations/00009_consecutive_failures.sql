-- +goose Up
-- Track consecutive connection test failures per integration.
-- Used by the RecoveryService for exponential backoff probing.
ALTER TABLE integration_configs ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN before 3.35.0; recreate the table
-- without the column. Since this is a non-critical metadata column,
-- data loss on downgrade is acceptable.
CREATE TABLE integration_configs_backup AS SELECT
    id, type, name, url, api_key, enabled, media_size_bytes, media_count,
    last_sync, last_error, collection_deletion, show_level_only, created_at, updated_at
FROM integration_configs;
DROP TABLE integration_configs;
ALTER TABLE integration_configs_backup RENAME TO integration_configs;
