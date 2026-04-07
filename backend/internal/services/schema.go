package services

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"capacitarr/internal/db"
)

// SchemaError describes a single schema validation failure.
type SchemaError struct {
	Table   string
	Column  string
	Problem string // "missing_table", "missing_column"
}

// String returns a human-readable description of the error.
func (e SchemaError) String() string {
	if e.Column != "" {
		return fmt.Sprintf("%s: %s.%s", e.Problem, e.Table, e.Column)
	}
	return fmt.Sprintf("%s: %s", e.Problem, e.Table)
}

// SchemaReport summarizes the results of a schema validation and repair cycle.
type SchemaReport struct {
	IntegrityOK    bool
	FixupsApplied  []string      // e.g., "renamed engine_run_stats.flagged → candidates"
	RepairsApplied []string      // e.g., "created table media_cache"
	Errors         []SchemaError // non-empty = fatal, app must not start
}

// SchemaService validates database integrity and schema correctness at startup.
// It is a bootstrap component — not added to the service registry. It runs once
// during startup (after Goose migrations, before NewRegistry()), performs
// validation and repair, then is discarded.
type SchemaService struct {
	database *gorm.DB
}

// NewSchemaService creates a SchemaService for startup schema validation.
func NewSchemaService(database *gorm.DB) *SchemaService {
	return &SchemaService{database: database}
}

// allModels lists every GORM model struct used in the application. Used by
// repair() for AutoMigrate and by validate() for schema comparison. New models
// must be appended here — the schema_test.go TestAllModelsComplete test verifies
// this list stays in sync with the actual database tables.
var allModels = []any{
	&db.AuthConfig{},
	&db.DiskGroup{},
	&db.IntegrationConfig{},
	&db.DiskGroupIntegration{},
	&db.LibraryHistory{},
	&db.PreferenceSet{},
	&db.ScoringFactorWeight{},
	&db.CustomRule{},
	&db.ApprovalQueueItem{},
	&db.SunsetQueueItem{},
	&db.AuditLogEntry{},
	&db.EngineRunStats{},
	&db.LifetimeStats{},
	&db.NotificationConfig{},
	&db.ActivityEvent{},
	&db.RollupState{},
	&db.MediaCache{},
	&db.MediaServerMapping{},
}

// ValidateAndRepair runs the full startup schema validation sequence:
//  1. PRAGMA integrity_check — detect SQLite file corruption
//  2. Idempotent DDL fixups — absorbed from db/migrate.go
//  3. AutoMigrate — additive repair for missing tables/columns
//  4. Schema validation — verify all GORM models match the live schema
//
// Returns a SchemaReport. If report.Errors is non-empty, the app must not start.
func (s *SchemaService) ValidateAndRepair() (*SchemaReport, error) {
	report := &SchemaReport{}

	// Step 1: PRAGMA integrity_check
	if err := s.checkIntegrity(); err != nil {
		report.IntegrityOK = false
		return report, fmt.Errorf("database integrity check failed: %w", err)
	}
	report.IntegrityOK = true

	// Step 2: Idempotent DDL fixups (absorbed from db/migrate.go)
	fixups, err := s.runFixups()
	if err != nil {
		return report, fmt.Errorf("post-migration fixups failed: %w", err)
	}
	report.FixupsApplied = fixups

	// Step 3: AutoMigrate — additive repair
	repairs := s.repair()
	report.RepairsApplied = repairs

	// Step 4: Schema validation
	report.Errors = s.validate()

	return report, nil
}

// checkIntegrity runs PRAGMA integrity_check and returns an error if the
// database file is corrupt.
func (s *SchemaService) checkIntegrity() error {
	var result string
	if err := s.database.Raw("PRAGMA integrity_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("PRAGMA integrity_check query failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("PRAGMA integrity_check returned: %s", result)
	}
	slog.Info("Database integrity check passed", "component", "schema")
	return nil
}

// runFixups executes idempotent post-migration DDL fixups that were previously
// in db/migrate.go. These handle column renames and additions that can't be
// expressed as conditional DDL in pure Goose SQL migrations.
func (s *SchemaService) runFixups() ([]string, error) {
	sqlDB, err := s.database.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	var applied []string

	// Fixup 1: engine_run_stats — rename flagged→candidates, add queued column
	fixup1, err := s.fixupEngineRunStats(sqlDB)
	if err != nil {
		return applied, fmt.Errorf("fixup engine_run_stats: %w", err)
	}
	applied = append(applied, fixup1...)

	// Fixup 2: preference_sets — rename execution_mode→default_disk_group_mode
	fixup2, err := s.fixupDefaultDiskGroupModeRename(sqlDB)
	if err != nil {
		return applied, fmt.Errorf("fixup default_disk_group_mode: %w", err)
	}
	applied = append(applied, fixup2...)

	return applied, nil
}

// fixupEngineRunStats applies idempotent schema changes to engine_run_stats:
//   - Renames "flagged" column to "candidates" (if flagged still exists)
//   - Adds "queued" column (if it doesn't exist yet)
func (s *SchemaService) fixupEngineRunStats(sqlDB *sql.DB) ([]string, error) {
	hasFlagged := s.hasColumn(sqlDB, "engine_run_stats", "flagged")
	hasCandidates := s.hasColumn(sqlDB, "engine_run_stats", "candidates")
	hasQueued := s.hasColumn(sqlDB, "engine_run_stats", "queued")

	ctx := context.Background()
	var applied []string

	if hasFlagged && !hasCandidates {
		slog.Info("Renaming engine_run_stats.flagged → candidates", "component", "schema")
		if _, err := sqlDB.ExecContext(ctx, "ALTER TABLE engine_run_stats RENAME COLUMN flagged TO candidates"); err != nil {
			return applied, fmt.Errorf("rename flagged→candidates: %w", err)
		}
		applied = append(applied, "renamed engine_run_stats.flagged → candidates")
	}

	if !hasQueued {
		slog.Info("Adding engine_run_stats.queued column", "component", "schema")
		if _, err := sqlDB.ExecContext(ctx, "ALTER TABLE engine_run_stats ADD COLUMN queued INTEGER NOT NULL DEFAULT 0"); err != nil {
			return applied, fmt.Errorf("add queued column: %w", err)
		}
		applied = append(applied, "added engine_run_stats.queued column")
	}

	return applied, nil
}

// fixupDefaultDiskGroupModeRename renames execution_mode → default_disk_group_mode
// on preference_sets and resets the value to "dry-run". This is part of the 3.0
// migration: execution mode moves from a global preference to a per-disk-group field.
func (s *SchemaService) fixupDefaultDiskGroupModeRename(sqlDB *sql.DB) ([]string, error) {
	hasOld := s.hasColumn(sqlDB, "preference_sets", "execution_mode")
	hasNew := s.hasColumn(sqlDB, "preference_sets", "default_disk_group_mode")

	if !hasOld || hasNew {
		return nil, nil
	}

	ctx := context.Background()
	var applied []string

	slog.Info("Renaming preference_sets.execution_mode → default_disk_group_mode", "component", "schema")
	if _, err := sqlDB.ExecContext(ctx, "ALTER TABLE preference_sets RENAME COLUMN execution_mode TO default_disk_group_mode"); err != nil {
		return applied, fmt.Errorf("rename execution_mode→default_disk_group_mode: %w", err)
	}
	applied = append(applied, "renamed preference_sets.execution_mode → default_disk_group_mode")

	slog.Info("Resetting default_disk_group_mode to dry-run for 3.0 upgrade safety", "component", "schema")
	if _, err := sqlDB.ExecContext(ctx, "UPDATE preference_sets SET default_disk_group_mode = 'dry-run'"); err != nil {
		return applied, fmt.Errorf("reset default_disk_group_mode to dry-run: %w", err)
	}
	applied = append(applied, "reset default_disk_group_mode to dry-run")

	return applied, nil
}

// hasColumn checks if a table has a specific column using PRAGMA table_info.
// This is a generalized version of the old hasColumnInTable() from db/migrate.go,
// which required a hardcoded tableColumnCheckers registry. This version works
// for any table name.
func (s *SchemaService) hasColumn(sqlDB *sql.DB, tableName, column string) bool {
	// Use fmt.Sprintf for the PRAGMA since PRAGMA doesn't support ? placeholders.
	// tableName is always a hardcoded string from within the service, never user input.
	rows, err := sqlDB.QueryContext(context.Background(), fmt.Sprintf("PRAGMA table_info(%s)", tableName)) //nolint:gosec // nosemgrep
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

// repair creates missing tables using AutoMigrate. Only tables that do NOT
// already exist are passed to AutoMigrate. Tables that exist are managed by
// Goose migrations and must NOT be touched — GORM's AutoMigrate on SQLite
// triggers destructive temp-table rebuilds when it detects DDL differences
// between GORM tags and the Goose-created schema (e.g., NOT NULL, DEFAULT
// clauses, comment syntax). This causes data loss: the rebuild copies only
// columns it knows about, resetting user-customized values (like thresholds)
// to schema defaults.
func (s *SchemaService) repair() []string {
	migrator := s.database.Migrator()
	var repairs []string

	// Collect only models whose tables are missing
	var missingModels []any
	for _, model := range allModels {
		if !migrator.HasTable(model) {
			missingModels = append(missingModels, model)
		}
	}

	if len(missingModels) == 0 {
		return repairs
	}

	// Run AutoMigrate only on missing tables
	if err := s.database.AutoMigrate(missingModels...); err != nil {
		slog.Error("AutoMigrate failed", "component", "schema", "error", err)
		return repairs
	}

	// Report what was created
	for _, model := range missingModels {
		name := tableName(model)
		if migrator.HasTable(model) {
			repairs = append(repairs, fmt.Sprintf("created table %s", name))
		}
	}

	if len(repairs) > 0 {
		slog.Info("AutoMigrate created missing tables", "component", "schema", "repairs", strings.Join(repairs, ", "))
	}

	return repairs
}

// validate checks that every GORM model's table and columns exist in the live
// database. Returns a slice of errors — if non-empty, the app must not start.
// Extra columns (present in DB but not in GORM model) are logged as warnings
// but not treated as errors.
func (s *SchemaService) validate() []SchemaError {
	migrator := s.database.Migrator()
	var errors []SchemaError

	for _, model := range allModels {
		name := tableName(model)

		if !migrator.HasTable(model) {
			errors = append(errors, SchemaError{
				Table:   name,
				Problem: "missing_table",
			})
			continue // no point checking columns on a missing table
		}

		// Check each field in the GORM model has a corresponding column
		stmt := &gorm.Statement{DB: s.database}
		if err := stmt.Parse(model); err != nil {
			slog.Warn("Failed to parse GORM model for validation", "component", "schema", "model", name, "error", err)
			continue
		}

		for _, field := range stmt.Schema.Fields {
			// Skip fields that don't map to database columns
			if field.DBName == "" || field.DBName == "-" {
				continue
			}

			if !migrator.HasColumn(model, field.DBName) {
				errors = append(errors, SchemaError{
					Table:   name,
					Column:  field.DBName,
					Problem: "missing_column",
				})
			}
		}
	}

	return errors
}

// tableName extracts the GORM table name for a model, respecting any custom
// TableName() method.
func tableName(model any) string {
	if tabler, ok := model.(interface{ TableName() string }); ok {
		return tabler.TableName()
	}
	// Fallback: let GORM figure it out
	return fmt.Sprintf("%T", model)
}
