-- +goose Up
CREATE TABLE IF NOT EXISTS engine_run_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    evaluated INTEGER NOT NULL DEFAULT 0,
    flagged INTEGER NOT NULL DEFAULT 0,
    freed_bytes INTEGER NOT NULL DEFAULT 0,
    execution_mode TEXT NOT NULL DEFAULT 'dry-run',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_engine_run_stats_run_at ON engine_run_stats(run_at);

-- +goose Down
DROP INDEX IF EXISTS idx_engine_run_stats_run_at;
DROP TABLE IF EXISTS engine_run_stats;
