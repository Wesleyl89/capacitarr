-- +goose Up
CREATE TABLE IF NOT EXISTS lifetime_stats (
    id INTEGER PRIMARY KEY DEFAULT 1,
    total_bytes_reclaimed INTEGER NOT NULL DEFAULT 0,
    total_items_removed INTEGER NOT NULL DEFAULT 0,
    total_engine_runs INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME
);

INSERT OR IGNORE INTO lifetime_stats (id) VALUES (1);

-- +goose Down
DROP TABLE IF EXISTS lifetime_stats;
