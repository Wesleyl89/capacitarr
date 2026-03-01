-- +goose Up
-- Add tiebreaker_method column to preference_sets.
-- For existing databases that had AutoMigrate, this column may not exist yet.
-- For fresh databases, the baseline (00001) already excludes it so this adds it.
ALTER TABLE preference_sets ADD COLUMN tiebreaker_method TEXT NOT NULL DEFAULT 'size_desc';

-- +goose Down
-- SQLite 3.35.0+ supports DROP COLUMN; older versions cannot reverse this.
ALTER TABLE preference_sets DROP COLUMN tiebreaker_method;
