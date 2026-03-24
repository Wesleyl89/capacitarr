-- +goose Up
-- Add show_level_only toggle for Sonarr integrations.
-- When enabled, the poller evaluates entire shows instead of individual seasons.

ALTER TABLE integration_configs ADD COLUMN show_level_only INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN before 3.35.0, but goose handles this
-- by recreating the table internally when needed.  For newer SQLite versions
-- this works directly.

ALTER TABLE integration_configs DROP COLUMN show_level_only;
