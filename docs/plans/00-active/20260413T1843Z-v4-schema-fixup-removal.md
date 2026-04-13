# v4: Remove Legacy SchemaService Go Fixups

> **Status:** Pending (v4 prerequisite)
> **Created:** 2026-04-13
> **Category:** Architecture / Database
> **Blocked by:** v4 development starting

## Summary

Two Go-level DDL fixups in `SchemaService.runFixups()` handle schema changes that couldn't be expressed as conditional SQL in Goose migrations. Both are no-ops for any database that has run v3 at least once. They should be removed in v4 to eliminate dead code and the unnecessary `PRAGMA table_info` calls on every startup.

## Background

### Fixup 1: `fixupEngineRunStats` (`schema.go:155`)

**What it does:**
- Renames `engine_run_stats.flagged` to `candidates`
- Adds `engine_run_stats.queued` column

**How it came about:** In commit `293a306` the baseline migration (`00001_v2_baseline.sql`) was edited in-place to use the new column names instead of creating an incremental migration. Existing databases that had already applied 00001 were stuck with the old `flagged` column. The Go fixup (commit `569d7e7`) was added to patch them up conditionally.

### Fixup 2: `fixupDefaultDiskGroupModeRename` (`schema.go:185`)

**What it does:**
- Renames `preference_sets.execution_mode` to `default_disk_group_mode`
- Resets the value to `dry-run` for upgrade safety

**How it came about:** Migration `00006_sunset_mode.sql` (v3.0) moved execution mode from a global preference to per-disk-group. The column rename requires conditional logic (check if old name exists, skip if already renamed) which SQLite Goose SQL migrations cannot express — there is no `IF EXISTS` variant of `ALTER TABLE RENAME COLUMN`. The fixup was added in commit `169af52` alongside the sunset mode feature.

### Why they're safe to remove in v4

Every possible upgrade path reaches v3 with correct column names:

| Starting version | Fixup 1 | Fixup 2 |
|---|---|---|
| Early v2 (pre-rename) -> v3 | Applied on first v3 startup | Applied on first v3 startup |
| Late v2 (post-rename) -> v3 | Already no-op | Applied on first v3 startup |
| Fresh v3 install | No-op (baseline correct) | No-op (baseline correct) |
| Any v3 -> v4 | **No-op** | **No-op** |

Since v4 will require v3 as a prerequisite (sequential major version upgrade), both fixups are guaranteed dead code by the time v4 runs.

## Steps

### Step 1: Add v3 version gate to v4 startup

Before Goose migrations run, check `schema_info.schema_family`. If the value is not `v3`, abort startup with a clear message:

```
Fatal: Capacitarr v4 requires upgrading from v3 first.
Your database is on schema family '<value>'.
Please install Capacitarr v3.x, start it once to complete migrations,
then upgrade to v4.
```

This gate ensures the fixups have already run (or were never needed) before v4 code executes.

### Step 2: Remove fixup methods from SchemaService

Delete from `backend/internal/services/schema.go`:
- `fixupEngineRunStats()` method (~25 lines)
- `fixupDefaultDiskGroupModeRename()` method (~25 lines)
- `hasColumn()` helper method (~25 lines) — verify no other callers remain before removing

Update `runFixups()` to either:
- Remove it entirely if no new v4 fixups are needed
- Keep the shell if new v4 fixups are being added

### Step 3: Update `db.go` test helpers

`backend/internal/db/db.go` contains `applyTestFixups()` which mirrors the SchemaService fixups for test databases. Remove the corresponding fixup logic there and in any test files that reference it.

### Step 4: Update tests

- `backend/internal/services/schema_test.go` — remove or update any tests that exercise the removed fixup methods
- `backend/internal/db/migrate_test.go` — verify `TestMigrations_UpDown` still passes (it should, since the fixups are orthogonal to Goose)

### Step 5: Consider v4 baseline consolidation (optional)

If v4 introduces breaking schema changes, this is the natural moment to consolidate migrations 00001-000XX into a single `00001_v4_baseline.sql`. The version gate from Step 1 makes this safe — no user will run the v4 baseline against a pre-v3 database.

### Step 6: Update migration `00006_sunset_mode.sql` comment

Remove or update the comment on line 5-6 that says:
```sql
-- The global execution_mode column rename is handled by a Go fixup in migrate.go
-- because SQLite column renames require conditional logic.
```

This comment references code that no longer exists after the removal.

## Verification

- `make ci` passes
- `TestMigrations_UpDown` passes (up/down/up cycle)
- `TestMigrations_Idempotent` passes
- `TestSchemaValidateAndRepair_ValidSchema` passes
- Fresh v4 install creates correct schema
- v3 -> v4 upgrade path works (version gate accepts `v3`, migrations run cleanly)
- Pre-v3 database triggers the version gate abort message
