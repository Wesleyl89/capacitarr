-- +goose Up
ALTER TABLE disk_groups ADD COLUMN stale_since DATETIME DEFAULT NULL;
ALTER TABLE preference_sets ADD COLUMN disk_group_grace_period_days INTEGER NOT NULL DEFAULT 7;

-- +goose Down
-- SQLite does not support DROP COLUMN before 3.35.0; columns are harmless if unused.
