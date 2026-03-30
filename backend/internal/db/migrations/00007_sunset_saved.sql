-- +goose Up
-- Add "saved by popular demand" support to sunset queue and preferences.
-- Items whose score drops during the countdown are transitioned to "saved"
-- status instead of being deleted.

-- Add rescore/saved preferences to preference_sets
ALTER TABLE preference_sets ADD COLUMN sunset_rescore_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE preference_sets ADD COLUMN saved_duration_days INTEGER NOT NULL DEFAULT 7;
ALTER TABLE preference_sets ADD COLUMN saved_label TEXT NOT NULL DEFAULT 'capacitarr-saved';

-- Add status tracking to sunset_queue
ALTER TABLE sunset_queue ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE sunset_queue ADD COLUMN saved_at DATETIME;
ALTER TABLE sunset_queue ADD COLUMN saved_score REAL NOT NULL DEFAULT 0;
ALTER TABLE sunset_queue ADD COLUMN saved_reason TEXT;

-- +goose Down
ALTER TABLE sunset_queue DROP COLUMN saved_reason;
ALTER TABLE sunset_queue DROP COLUMN saved_score;
ALTER TABLE sunset_queue DROP COLUMN saved_at;
ALTER TABLE sunset_queue DROP COLUMN status;
ALTER TABLE preference_sets DROP COLUMN saved_label;
ALTER TABLE preference_sets DROP COLUMN saved_duration_days;
ALTER TABLE preference_sets DROP COLUMN sunset_rescore_enabled;
