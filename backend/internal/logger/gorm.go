package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

// defaultSlowThreshold is the duration above which a SQL query is logged as
// slow at WARN level instead of DEBUG.
const defaultSlowThreshold = 200 * time.Millisecond

// SlogAdapter implements gorm.io/gorm/logger.Interface and routes all GORM
// log output through log/slog. This unifies GORM's SQL query logging with
// the application's structured JSON logging, ensuring every log line has a
// proper level, timestamp, and component field.
type SlogAdapter struct {
	slowThreshold time.Duration
	level         gormlogger.LogLevel
}

// NewGormLogger creates a SlogAdapter with the given slow-query threshold.
// Queries exceeding this duration are logged at WARN level; all others at
// DEBUG. Pass 0 to use the default threshold (200ms).
func NewGormLogger(slowThreshold time.Duration) *SlogAdapter {
	if slowThreshold <= 0 {
		slowThreshold = defaultSlowThreshold
	}
	return &SlogAdapter{
		slowThreshold: slowThreshold,
		level:         gormlogger.Warn,
	}
}

// LogMode returns a new SlogAdapter with the given GORM log level. This
// follows GORM's convention of returning a new instance rather than mutating.
func (s *SlogAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &SlogAdapter{
		slowThreshold: s.slowThreshold,
		level:         level,
	}
}

// Info logs a GORM informational message at slog DEBUG level. GORM's "info"
// messages are verbose internal details (e.g., connection pool stats) that
// map naturally to debug-level output.
func (s *SlogAdapter) Info(_ context.Context, msg string, args ...interface{}) {
	if s.level >= gormlogger.Info {
		slog.Debug(fmt.Sprintf(msg, args...), "component", "gorm", "file", utils.FileWithLineNum())
	}
}

// Warn logs a GORM warning at slog WARN level.
func (s *SlogAdapter) Warn(_ context.Context, msg string, args ...interface{}) {
	if s.level >= gormlogger.Warn {
		slog.Warn(fmt.Sprintf(msg, args...), "component", "gorm", "file", utils.FileWithLineNum())
	}
}

// Error logs a GORM error at slog ERROR level.
func (s *SlogAdapter) Error(_ context.Context, msg string, args ...interface{}) {
	if s.level >= gormlogger.Error {
		slog.Error(fmt.Sprintf(msg, args...), "component", "gorm", "file", utils.FileWithLineNum())
	}
}

// Trace logs SQL query execution. It is called by GORM after every database
// operation with the query start time, a function that returns the SQL and
// row count, and any error that occurred.
//
// Level mapping:
//   - Error (not ErrRecordNotFound): slog.Error
//   - Slow query (> slowThreshold):  slog.Warn
//   - Normal query:                  slog.Debug
func (s *SlogAdapter) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if s.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	elapsedMs := float64(elapsed.Nanoseconds()) / 1e6
	sql, rows := fc()
	file := utils.FileWithLineNum()

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && s.level >= gormlogger.Error:
		slog.Error("SQL error",
			"component", "gorm",
			"file", file,
			"error", err.Error(),
			"duration_ms", elapsedMs,
			"rows", rows,
			"sql", sql,
		)
	case elapsed > s.slowThreshold && s.slowThreshold > 0 && s.level >= gormlogger.Warn:
		slog.Warn("slow SQL query",
			"component", "gorm",
			"file", file,
			"duration_ms", elapsedMs,
			"rows", rows,
			"threshold_ms", float64(s.slowThreshold.Nanoseconds())/1e6,
			"sql", sql,
		)
	case s.level >= gormlogger.Info:
		slog.Debug("SQL query",
			"component", "gorm",
			"file", file,
			"duration_ms", elapsedMs,
			"rows", rows,
			"sql", sql,
		)
	}
}
