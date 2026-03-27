package db

import (
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"strings"
	"time"
)

// retryMaxAttempts is the default maximum number of retry attempts for
// WithRetry when SQLite returns SQLITE_BUSY.
const retryMaxAttempts = 3

// retryBaseDelay is the initial backoff delay between retry attempts.
// Each subsequent attempt doubles the delay (with jitter).
const retryBaseDelay = 50 * time.Millisecond

// WithRetry wraps a function call with exponential backoff retry on
// SQLITE_BUSY errors. This provides application-level retry at the service
// call level where full operation context is available, rather than inside
// GORM's callback pipeline where only raw SQL is available.
//
// Usage:
//
//	err := db.WithRetry(func() error {
//	    return s.db.Create(&row).Error
//	}, 3)
func WithRetry(fn func() error, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = retryMaxAttempts
	}

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil || !isSQLiteBusy(err) {
			return err
		}

		if attempt >= maxAttempts {
			slog.Error("SQLite busy retry exhausted",
				"component", "db",
				"attempts", attempt,
				"error", err,
			)
			return err
		}

		// Exponential backoff with jitter
		delay := retryBaseDelay * time.Duration(1<<(attempt-1))
		jitter := time.Duration(cryptoRandInt64(int64(delay / 2)))
		delay += jitter

		slog.Warn("SQLite busy, retrying operation",
			"component", "db",
			"attempt", attempt,
			"maxAttempts", maxAttempts,
			"backoff", delay,
		)

		time.Sleep(delay)
	}
	return err
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
