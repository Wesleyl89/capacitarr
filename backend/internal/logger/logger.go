// Package logger configures structured logging for the application.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// LogLevel is a global variable that lets us change the log level dynamically.
var LogLevel = new(slog.LevelVar)

// Init initializes the global slog logger with a JSON handler and the dynamic LogLevel.
func Init(debug bool) {
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

// SetLevel parses a string level and updates the global LogLevel var.
func SetLevel(level string) {
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
		// Default to info if unrecognized
		LogLevel.Set(slog.LevelInfo)
	}
}
