-- +goose Up
-- Persistent TMDb → media server native ID mapping table.
-- Replaces the ephemeral in-memory maps built by BuildTMDbToNativeIDMaps().
-- Populated during engine poll cycles; survives media server downtime.
CREATE TABLE media_server_mappings (
    tmdb_id          INTEGER NOT NULL,
    integration_id   INTEGER NOT NULL,
    native_id        TEXT NOT NULL,
    media_type       TEXT NOT NULL DEFAULT 'movie',
    title            TEXT NOT NULL DEFAULT '',
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_id, integration_id),
    FOREIGN KEY (integration_id) REFERENCES integration_configs(id) ON DELETE CASCADE
);
CREATE INDEX idx_msm_integration ON media_server_mappings(integration_id);
CREATE INDEX idx_msm_updated ON media_server_mappings(updated_at);

-- +goose Down
DROP TABLE IF EXISTS media_server_mappings;
