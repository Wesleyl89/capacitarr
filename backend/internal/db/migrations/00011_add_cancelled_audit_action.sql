-- +goose Up
-- Recreate audit_log table to extend the action CHECK constraint with 'cancelled'.
-- SQLite does not support ALTER TABLE ... DROP CONSTRAINT, so we use the
-- recommended rename-copy-drop pattern.

ALTER TABLE audit_log RENAME TO audit_log_old;

CREATE TABLE audit_log (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    media_name     TEXT    NOT NULL,
    media_type     TEXT    NOT NULL,
    reason         TEXT    NOT NULL,
    score_details  TEXT,
    action         TEXT    NOT NULL CHECK(action IN ('deleted','dry_run','dry_delete','cancelled')),
    size_bytes     INTEGER NOT NULL DEFAULT 0,
    integration_id INTEGER REFERENCES integration_configs(id) ON DELETE SET NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO audit_log SELECT * FROM audit_log_old;

DROP TABLE audit_log_old;

CREATE INDEX idx_audit_log_media_name ON audit_log(media_name);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

-- +goose Down
ALTER TABLE audit_log RENAME TO audit_log_old;

CREATE TABLE audit_log (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    media_name     TEXT    NOT NULL,
    media_type     TEXT    NOT NULL,
    reason         TEXT    NOT NULL,
    score_details  TEXT,
    action         TEXT    NOT NULL CHECK(action IN ('deleted','dry_run','dry_delete')),
    size_bytes     INTEGER NOT NULL DEFAULT 0,
    integration_id INTEGER REFERENCES integration_configs(id) ON DELETE SET NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO audit_log SELECT * FROM audit_log_old WHERE action != 'cancelled';

DROP TABLE audit_log_old;

CREATE INDEX idx_audit_log_media_name ON audit_log(media_name);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);
