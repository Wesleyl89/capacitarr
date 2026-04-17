-- +goose Up
-- Add add_import_exclusion toggle for *arr integrations.
-- When enabled (default true), deleted items are added to the *arr server's
-- import exclusion list to prevent automatic re-addition by import lists.

ALTER TABLE integration_configs ADD COLUMN add_import_exclusion INTEGER NOT NULL DEFAULT 1;

-- +goose Down

ALTER TABLE integration_configs DROP COLUMN add_import_exclusion;
