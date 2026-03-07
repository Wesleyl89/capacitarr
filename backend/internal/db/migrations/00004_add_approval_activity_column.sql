-- +goose Up
-- Add the on_approval_activity subscription column for approval queue notifications.
ALTER TABLE notification_configs ADD COLUMN on_approval_activity INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE notification_configs DROP COLUMN on_approval_activity;
