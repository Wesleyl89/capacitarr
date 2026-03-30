// Package logger configures structured logging for the application.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// LogLevel is a global variable that lets us change the log level dynamically.
var LogLevel = new(slog.LevelVar)

// debugEnv tracks whether DEBUG=true was set at startup. When true, the log
// level is pinned to debug regardless of the database preference — the env
// var acts as a floor that SetLevel cannot raise above.
var debugEnv bool

// Init initializes the global slog logger with a JSON handler and the dynamic LogLevel.
func Init(debug bool) {
	debugEnv = debug

	if debug {
		LogLevel.Set(slog.LevelDebug)
	} else {
		LogLevel.Set(slog.LevelInfo)
	}

	opts := &slog.HandlerOptions{
		Level: LogLevel,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

// DebugOverride returns true when DEBUG=true was set at startup, meaning
// the log level is pinned to debug regardless of the database preference.
func DebugOverride() bool {
	return debugEnv
}

// SetLevel parses a string level and updates the global LogLevel var.
// If DEBUG=true was set at startup, the level is pinned to debug — database
// preferences cannot raise it above debug.
func SetLevel(level string) {
	if debugEnv {
		LogLevel.Set(slog.LevelDebug)
		return
	}

	switch strings.ToLower(level) {
	case "debug":
		LogLevel.Set(slog.LevelDebug)
	case "info":
		LogLevel.Set(slog.LevelInfo)
	case "warn":
		LogLevel.Set(slog.LevelWarn)
	case "error":
		LogLevel.Set(slog.LevelError)
	default:
		LogLevel.Set(slog.LevelInfo)
	}
}
