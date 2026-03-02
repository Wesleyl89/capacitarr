package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// RunMigrations applies all pending Goose migrations using the embedded SQL files.
// For existing databases the baseline migration (00001) is a no-op because every
// statement uses IF NOT EXISTS / INSERT OR IGNORE.  For fresh installs it creates
// the full schema.
func RunMigrations(sqlDB *sql.DB) error {
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	slog.Info("Database migrations applied successfully", "component", "db")
	return nil
}
