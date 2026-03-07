-- +goose Up
-- Replace per-event notification subscription columns with digest/alert toggles.
-- New columns default to 1 (enabled) so existing channels get all new notification types.

-- Add new subscription columns
ALTER TABLE notification_configs ADD COLUMN on_cycle_digest INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_error INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_mode_changed INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_server_started INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_update_available INTEGER NOT NULL DEFAULT 1;

-- Drop old subscription columns (ncruces/go-sqlite3 bundles SQLite 3.35+)
ALTER TABLE notification_configs DROP COLUMN on_deletion_executed;
ALTER TABLE notification_configs DROP COLUMN on_engine_complete;
ALTER TABLE notification_configs DROP COLUMN on_engine_error;

-- +goose Down
-- Reverse: add old columns back, remove new columns
ALTER TABLE notification_configs ADD COLUMN on_deletion_executed INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_engine_complete INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_configs ADD COLUMN on_engine_error INTEGER NOT NULL DEFAULT 1;

ALTER TABLE notification_configs DROP COLUMN on_cycle_digest;
ALTER TABLE notification_configs DROP COLUMN on_error;
ALTER TABLE notification_configs DROP COLUMN on_mode_changed;
ALTER TABLE notification_configs DROP COLUMN on_server_started;
ALTER TABLE notification_configs DROP COLUMN on_update_available;
