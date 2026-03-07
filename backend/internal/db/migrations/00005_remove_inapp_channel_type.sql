-- +goose Up
-- Remove any user-created "inapp" notification channel configs.
-- In-app notifications are now always-on and do not require a channel config.
DELETE FROM notification_configs WHERE type = 'inapp';

-- +goose Down
-- No rollback needed — in-app channels can be recreated manually if needed.
