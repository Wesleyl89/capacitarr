-- +goose Up
-- Track per-disk-group execution modes on each engine run.
-- JSON map of diskGroupID → mode (e.g. {"1":"auto","2":"sunset"}).
ALTER TABLE engine_run_stats ADD COLUMN disk_group_modes TEXT DEFAULT '' NOT NULL;

-- +goose Down
-- SQLite 3.35+ supports ALTER TABLE DROP COLUMN
ALTER TABLE engine_run_stats DROP COLUMN disk_group_modes;
