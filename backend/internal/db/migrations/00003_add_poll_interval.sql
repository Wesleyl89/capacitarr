-- +goose Up
ALTER TABLE preference_sets ADD COLUMN poll_interval_seconds INTEGER NOT NULL DEFAULT 300;

-- +goose Down
-- SQLite does not support DROP COLUMN prior to 3.35.0; this is a best-effort rollback.
-- For older SQLite versions this migration cannot be fully reversed.
ALTER TABLE preference_sets DROP COLUMN poll_interval_seconds;
