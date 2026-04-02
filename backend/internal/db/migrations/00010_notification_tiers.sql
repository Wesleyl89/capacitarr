-- +goose Up
-- Notification tiers: replace 10 individual boolean toggles with a single
-- notification_level enum (off / critical / important / normal / verbose) plus
-- nullable per-event override columns. Existing boolean combinations are
-- mapped to the closest tier, with deviations preserved as overrides.

-- Step 1: Repair corrupted on_sunset_activity (PartialUpdate bug reset it to false)
UPDATE notification_configs SET on_sunset_activity = 1;

-- Step 2: Add notification_level and override columns
ALTER TABLE notification_configs ADD COLUMN notification_level TEXT NOT NULL DEFAULT 'normal';
ALTER TABLE notification_configs ADD COLUMN override_cycle_digest INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_error INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_mode_changed INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_server_started INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_threshold_breach INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_update_available INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_approval_activity INTEGER DEFAULT NULL;
ALTER TABLE notification_configs ADD COLUMN override_integration_status INTEGER DEFAULT NULL;

-- Step 3: Map existing booleans to tiers
-- All false → off
UPDATE notification_configs SET notification_level = 'off'
WHERE on_cycle_digest = 0 AND on_error = 0 AND on_threshold_breach = 0
  AND on_mode_changed = 0 AND on_approval_activity = 0
  AND on_server_started = 0 AND on_update_available = 0
  AND on_dry_run_digest = 0 AND on_integration_status = 0;

-- All true → verbose (dry_run_digest=true means they want everything)
UPDATE notification_configs SET notification_level = 'verbose'
WHERE notification_level = 'normal'
  AND on_cycle_digest = 1 AND on_error = 1 AND on_threshold_breach = 1
  AND on_mode_changed = 1 AND on_approval_activity = 1
  AND on_server_started = 1 AND on_update_available = 1
  AND on_dry_run_digest = 1 AND on_integration_status = 1;

-- Error + threshold only → critical
UPDATE notification_configs SET notification_level = 'critical'
WHERE notification_level = 'normal'
  AND on_error = 1 AND on_threshold_breach = 1
  AND on_cycle_digest = 0 AND on_mode_changed = 0
  AND on_approval_activity = 0 AND on_server_started = 0
  AND on_update_available = 0 AND on_dry_run_digest = 0;

-- Everything remaining stays 'normal' (the default)

-- Step 4: Compute overrides — set override when boolean deviates from tier default.
--
-- Tier defaults:
--   off:       all false
--   critical:  error=1, threshold_breach=1, integration_status=1, rest=0
--   important: critical + mode_changed=1, approval_activity=1
--   normal:    important + cycle_digest=1, update_available=1, server_started=1
--   verbose:   all true (normal + dry_run_digest=1)

-- Overrides for 'off' tier (default: all false)
UPDATE notification_configs SET override_cycle_digest = 1       WHERE notification_level = 'off' AND on_cycle_digest = 1;
UPDATE notification_configs SET override_error = 1              WHERE notification_level = 'off' AND on_error = 1;
UPDATE notification_configs SET override_mode_changed = 1       WHERE notification_level = 'off' AND on_mode_changed = 1;
UPDATE notification_configs SET override_server_started = 1     WHERE notification_level = 'off' AND on_server_started = 1;
UPDATE notification_configs SET override_threshold_breach = 1   WHERE notification_level = 'off' AND on_threshold_breach = 1;
UPDATE notification_configs SET override_update_available = 1   WHERE notification_level = 'off' AND on_update_available = 1;
UPDATE notification_configs SET override_approval_activity = 1  WHERE notification_level = 'off' AND on_approval_activity = 1;
UPDATE notification_configs SET override_integration_status = 1 WHERE notification_level = 'off' AND on_integration_status = 1;

-- Overrides for 'critical' tier (default: error=1, threshold_breach=1, integration_status=1, rest=0)
UPDATE notification_configs SET override_error = 0              WHERE notification_level = 'critical' AND on_error = 0;
UPDATE notification_configs SET override_threshold_breach = 0   WHERE notification_level = 'critical' AND on_threshold_breach = 0;
UPDATE notification_configs SET override_integration_status = 0 WHERE notification_level = 'critical' AND on_integration_status = 0;
UPDATE notification_configs SET override_cycle_digest = 1       WHERE notification_level = 'critical' AND on_cycle_digest = 1;
UPDATE notification_configs SET override_mode_changed = 1       WHERE notification_level = 'critical' AND on_mode_changed = 1;
UPDATE notification_configs SET override_server_started = 1     WHERE notification_level = 'critical' AND on_server_started = 1;
UPDATE notification_configs SET override_update_available = 1   WHERE notification_level = 'critical' AND on_update_available = 1;
UPDATE notification_configs SET override_approval_activity = 1  WHERE notification_level = 'critical' AND on_approval_activity = 1;

-- Overrides for 'important' tier (default: critical + mode_changed=1, approval_activity=1)
UPDATE notification_configs SET override_error = 0              WHERE notification_level = 'important' AND on_error = 0;
UPDATE notification_configs SET override_threshold_breach = 0   WHERE notification_level = 'important' AND on_threshold_breach = 0;
UPDATE notification_configs SET override_integration_status = 0 WHERE notification_level = 'important' AND on_integration_status = 0;
UPDATE notification_configs SET override_mode_changed = 0       WHERE notification_level = 'important' AND on_mode_changed = 0;
UPDATE notification_configs SET override_approval_activity = 0  WHERE notification_level = 'important' AND on_approval_activity = 0;
UPDATE notification_configs SET override_cycle_digest = 1       WHERE notification_level = 'important' AND on_cycle_digest = 1;
UPDATE notification_configs SET override_server_started = 1     WHERE notification_level = 'important' AND on_server_started = 1;
UPDATE notification_configs SET override_update_available = 1   WHERE notification_level = 'important' AND on_update_available = 1;

-- Overrides for 'normal' tier (default: important + cycle_digest=1, update_available=1, server_started=1)
UPDATE notification_configs SET override_error = 0              WHERE notification_level = 'normal' AND on_error = 0;
UPDATE notification_configs SET override_threshold_breach = 0   WHERE notification_level = 'normal' AND on_threshold_breach = 0;
UPDATE notification_configs SET override_integration_status = 0 WHERE notification_level = 'normal' AND on_integration_status = 0;
UPDATE notification_configs SET override_mode_changed = 0       WHERE notification_level = 'normal' AND on_mode_changed = 0;
UPDATE notification_configs SET override_approval_activity = 0  WHERE notification_level = 'normal' AND on_approval_activity = 0;
UPDATE notification_configs SET override_cycle_digest = 0       WHERE notification_level = 'normal' AND on_cycle_digest = 0;
UPDATE notification_configs SET override_update_available = 0   WHERE notification_level = 'normal' AND on_update_available = 0;
UPDATE notification_configs SET override_server_started = 0     WHERE notification_level = 'normal' AND on_server_started = 0;

-- Overrides for 'verbose' tier (default: all true)
UPDATE notification_configs SET override_cycle_digest = 0       WHERE notification_level = 'verbose' AND on_cycle_digest = 0;
UPDATE notification_configs SET override_error = 0              WHERE notification_level = 'verbose' AND on_error = 0;
UPDATE notification_configs SET override_mode_changed = 0       WHERE notification_level = 'verbose' AND on_mode_changed = 0;
UPDATE notification_configs SET override_server_started = 0     WHERE notification_level = 'verbose' AND on_server_started = 0;
UPDATE notification_configs SET override_threshold_breach = 0   WHERE notification_level = 'verbose' AND on_threshold_breach = 0;
UPDATE notification_configs SET override_update_available = 0   WHERE notification_level = 'verbose' AND on_update_available = 0;
UPDATE notification_configs SET override_approval_activity = 0  WHERE notification_level = 'verbose' AND on_approval_activity = 0;
UPDATE notification_configs SET override_integration_status = 0 WHERE notification_level = 'verbose' AND on_integration_status = 0;

-- Step 5: Drop old boolean columns
ALTER TABLE notification_configs DROP COLUMN on_cycle_digest;
ALTER TABLE notification_configs DROP COLUMN on_dry_run_digest;
ALTER TABLE notification_configs DROP COLUMN on_error;
ALTER TABLE notification_configs DROP COLUMN on_mode_changed;
ALTER TABLE notification_configs DROP COLUMN on_server_started;
ALTER TABLE notification_configs DROP COLUMN on_threshold_breach;
ALTER TABLE notification_configs DROP COLUMN on_update_available;
ALTER TABLE notification_configs DROP COLUMN on_approval_activity;
ALTER TABLE notification_configs DROP COLUMN on_integration_status;
ALTER TABLE notification_configs DROP COLUMN on_sunset_activity;

-- +goose Down
-- Reverse: re-create boolean columns, map back from tier + overrides, drop new columns.

-- Step 1: Re-create old boolean columns with defaults
ALTER TABLE notification_configs ADD COLUMN on_cycle_digest INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_dry_run_digest INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_error INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_mode_changed INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_server_started INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_threshold_breach INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_update_available INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_approval_activity INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_integration_status INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notification_configs ADD COLUMN on_sunset_activity INTEGER NOT NULL DEFAULT 1;

-- Step 2: Populate booleans from tier defaults
-- off: all false
UPDATE notification_configs SET
    on_cycle_digest = 0, on_dry_run_digest = 0, on_error = 0,
    on_mode_changed = 0, on_server_started = 0, on_threshold_breach = 0,
    on_update_available = 0, on_approval_activity = 0, on_integration_status = 0,
    on_sunset_activity = 0
WHERE notification_level = 'off';

-- critical: error=1, threshold_breach=1, integration_status=1, rest=0
UPDATE notification_configs SET
    on_cycle_digest = 0, on_dry_run_digest = 0, on_error = 1,
    on_mode_changed = 0, on_server_started = 0, on_threshold_breach = 1,
    on_update_available = 0, on_approval_activity = 0, on_integration_status = 1,
    on_sunset_activity = 0
WHERE notification_level = 'critical';

-- important: critical + mode_changed=1, approval_activity=1
UPDATE notification_configs SET
    on_cycle_digest = 0, on_dry_run_digest = 0, on_error = 1,
    on_mode_changed = 1, on_server_started = 0, on_threshold_breach = 1,
    on_update_available = 0, on_approval_activity = 1, on_integration_status = 1,
    on_sunset_activity = 0
WHERE notification_level = 'important';

-- normal: important + cycle_digest=1, update_available=1, server_started=1
UPDATE notification_configs SET
    on_cycle_digest = 1, on_dry_run_digest = 0, on_error = 1,
    on_mode_changed = 1, on_server_started = 1, on_threshold_breach = 1,
    on_update_available = 1, on_approval_activity = 1, on_integration_status = 1,
    on_sunset_activity = 1
WHERE notification_level = 'normal';

-- verbose: all true
UPDATE notification_configs SET
    on_cycle_digest = 1, on_dry_run_digest = 1, on_error = 1,
    on_mode_changed = 1, on_server_started = 1, on_threshold_breach = 1,
    on_update_available = 1, on_approval_activity = 1, on_integration_status = 1,
    on_sunset_activity = 1
WHERE notification_level = 'verbose';

-- Step 3: Apply overrides back to booleans
UPDATE notification_configs SET on_cycle_digest = override_cycle_digest             WHERE override_cycle_digest IS NOT NULL;
UPDATE notification_configs SET on_error = override_error                           WHERE override_error IS NOT NULL;
UPDATE notification_configs SET on_mode_changed = override_mode_changed             WHERE override_mode_changed IS NOT NULL;
UPDATE notification_configs SET on_server_started = override_server_started         WHERE override_server_started IS NOT NULL;
UPDATE notification_configs SET on_threshold_breach = override_threshold_breach     WHERE override_threshold_breach IS NOT NULL;
UPDATE notification_configs SET on_update_available = override_update_available     WHERE override_update_available IS NOT NULL;
UPDATE notification_configs SET on_approval_activity = override_approval_activity   WHERE override_approval_activity IS NOT NULL;
UPDATE notification_configs SET on_integration_status = override_integration_status WHERE override_integration_status IS NOT NULL;

-- Step 4: Drop new columns
ALTER TABLE notification_configs DROP COLUMN notification_level;
ALTER TABLE notification_configs DROP COLUMN override_cycle_digest;
ALTER TABLE notification_configs DROP COLUMN override_error;
ALTER TABLE notification_configs DROP COLUMN override_mode_changed;
ALTER TABLE notification_configs DROP COLUMN override_server_started;
ALTER TABLE notification_configs DROP COLUMN override_threshold_breach;
ALTER TABLE notification_configs DROP COLUMN override_update_available;
ALTER TABLE notification_configs DROP COLUMN override_approval_activity;
ALTER TABLE notification_configs DROP COLUMN override_integration_status;
