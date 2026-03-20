-- +goose Up
-- Persistent media cache for restart recovery.
-- Stores a JSON snapshot of the preview result (scored media items + disk context)
-- so the dashboard and analytics have data immediately on startup without
-- waiting for the first engine run to complete.
-- This is a singleton table (id = 1) — each engine run replaces the row.

CREATE TABLE media_cache (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    preview_json TEXT     NOT NULL DEFAULT '{}',
    item_count   INTEGER  NOT NULL DEFAULT 0,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS media_cache;
