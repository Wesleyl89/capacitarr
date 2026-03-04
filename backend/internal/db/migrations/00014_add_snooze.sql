-- +goose Up
ALTER TABLE audit_logs ADD COLUMN snoozed_until DATETIME NULL;
ALTER TABLE preference_sets ADD COLUMN snooze_duration_hours INTEGER NOT NULL DEFAULT 24;

-- +goose Down
-- SQLite does not support DROP COLUMN; handled by recreation in future migrations.
