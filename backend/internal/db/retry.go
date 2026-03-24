package db

import (
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
)

// retryMaxAttempts is the maximum number of times a write operation is retried
// when SQLite returns SQLITE_BUSY ("database is locked"). With WAL mode and
// busy_timeout already handling most contention, this is a defense-in-depth
// measure for edge cases where the busy timeout expires.
const retryMaxAttempts = 3

// retryBaseDelay is the initial backoff delay between retry attempts.
// Each subsequent attempt doubles the delay (with jitter).
const retryBaseDelay = 50 * time.Millisecond

// RegisterRetryCallbacks installs GORM callbacks that automatically retry
// Create, Update, and Delete operations when SQLite returns "database is locked"
// (SQLITE_BUSY). This provides application-level retry as a defense-in-depth
// measure on top of WAL mode and busy_timeout.
//
// The retry uses exponential backoff with jitter to avoid thundering herd
// problems when multiple goroutines retry simultaneously.
func RegisterRetryCallbacks(database *gorm.DB) {
	_ = database.Callback().Create().After("gorm:create").Register("sqlite_retry:create", func(tx *gorm.DB) {
		retryOnBusy(tx, "create")
	})
	_ = database.Callback().Update().After("gorm:update").Register("sqlite_retry:update", func(tx *gorm.DB) {
		retryOnBusy(tx, "update")
	})
	_ = database.Callback().Delete().After("gorm:delete").Register("sqlite_retry:delete", func(tx *gorm.DB) {
		retryOnBusy(tx, "delete")
	})
}

// retryOnBusy checks if the current GORM operation failed with SQLITE_BUSY
// and retries it with exponential backoff. The retry count is tracked in the
// GORM statement's settings to prevent infinite recursion.
func retryOnBusy(tx *gorm.DB, operation string) {
	if tx.Error == nil {
		return
	}

	if !isSQLiteBusy(tx.Error) {
		return
	}

	// Get or initialize retry count from statement settings
	attempt := 1
	if val, ok := tx.Get("sqlite_retry_attempt"); ok {
		if a, ok := val.(int); ok {
			attempt = a
		}
	}

	if attempt >= retryMaxAttempts {
		slog.Error("SQLite busy retry exhausted",
			"component", "db",
			"operation", operation,
			"attempts", attempt,
			"error", tx.Error,
		)
		return
	}

	// Exponential backoff with jitter
	delay := retryBaseDelay * time.Duration(1<<(attempt-1))
	jitter := time.Duration(cryptoRandInt64(int64(delay / 2)))
	delay += jitter

	slog.Warn("SQLite busy, retrying operation",
		"component", "db",
		"operation", operation,
		"attempt", attempt,
		"maxAttempts", retryMaxAttempts,
		"backoff", delay,
	)

	time.Sleep(delay)

	// Clear the error and re-execute the statement
	tx.Error = nil
	tx.Set("sqlite_retry_attempt", attempt+1)

	// Re-execute the SQL statement
	result := tx.Session(&gorm.Session{NewDB: true}).Exec(
		tx.Statement.SQL.String(),
		tx.Statement.Vars...,
	)
	tx.Error = result.Error
	tx.RowsAffected = result.RowsAffected
}

// isSQLiteBusy checks if an error is a SQLite SQLITE_BUSY error.
// The ncruces/go-sqlite3 driver returns errors with "database is locked"
// in the message for SQLITE_BUSY (error code 5).
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "SQLITE_BUSY")
}

// cryptoRandInt64 returns a cryptographically random int64 in [0, bound).
// Falls back to 0 if crypto/rand fails (should never happen in practice).
func cryptoRandInt64(bound int64) int64 {
	if bound <= 0 {
		return 0
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0
	}
	// Mask the high bit to ensure a non-negative int64 without overflow.
	n := int64(binary.LittleEndian.Uint64(buf[:]) & 0x7FFFFFFFFFFFFFFF)
	return n % bound
}
