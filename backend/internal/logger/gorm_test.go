package logger

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// captureHandler is a slog.Handler that records all log records into a slice
// for test assertions. It is safe for concurrent use.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
	level   slog.Level
}

func newCaptureHandler(level slog.Level) *captureHandler {
	return &captureHandler{level: level}
}

func (h *captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) getRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]slog.Record, len(h.records))
	copy(cp, h.records)
	return cp
}

// installCapture replaces the default slog logger with one backed by a
// captureHandler and returns the handler. The caller must restore the
// previous logger when done (deferred via t.Cleanup).
func installCapture(t *testing.T, level slog.Level) *captureHandler {
	t.Helper()
	ch := newCaptureHandler(level)
	prev := slog.Default()
	slog.SetDefault(slog.New(ch))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return ch
}

// attrValue extracts the string value of a named attribute from a slog.Record.
func attrValue(r slog.Record, key string) string {
	var val string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value.String()
			return false
		}
		return true
	})
	return val
}

// attrFloat extracts the float64 value of a named attribute from a slog.Record.
func attrFloat(r slog.Record, key string) float64 {
	var val float64
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value.Float64()
			return false
		}
		return true
	})
	return val
}

func TestNewGormLogger_DefaultThreshold(t *testing.T) {
	l := NewGormLogger(0)
	if l.slowThreshold != defaultSlowThreshold {
		t.Errorf("expected default threshold %v, got %v", defaultSlowThreshold, l.slowThreshold)
	}
}

func TestNewGormLogger_CustomThreshold(t *testing.T) {
	l := NewGormLogger(500 * time.Millisecond)
	if l.slowThreshold != 500*time.Millisecond {
		t.Errorf("expected 500ms threshold, got %v", l.slowThreshold)
	}
}

func TestLogMode_ReturnsNewInstance(t *testing.T) {
	original := NewGormLogger(0)
	modified := original.LogMode(gormlogger.Info)

	// Must return a different instance
	if original == modified {
		t.Error("LogMode should return a new instance, got same pointer")
	}

	// Original should be unchanged
	if original.level != gormlogger.Warn {
		t.Errorf("original level changed: got %v, want %v", original.level, gormlogger.Warn)
	}

	// Modified should have the new level
	adapter, ok := modified.(*SlogAdapter)
	if !ok {
		t.Fatal("LogMode did not return *SlogAdapter")
	}
	if adapter.level != gormlogger.Info {
		t.Errorf("modified level: got %v, want %v", adapter.level, gormlogger.Info)
	}

	// Threshold should be preserved
	if adapter.slowThreshold != defaultSlowThreshold {
		t.Errorf("threshold not preserved: got %v, want %v", adapter.slowThreshold, defaultSlowThreshold)
	}
}

func TestInfo_LogsAtDebugLevel(t *testing.T) {
	ch := installCapture(t, slog.LevelDebug)
	l := NewGormLogger(0).LogMode(gormlogger.Info)

	l.Info(context.Background(), "connection pool stats: %d active", 5)

	records := ch.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelDebug {
		t.Errorf("expected DEBUG level, got %v", records[0].Level)
	}
	if records[0].Message != "connection pool stats: 5 active" {
		t.Errorf("unexpected message: %q", records[0].Message)
	}
	if v := attrValue(records[0], "component"); v != "gorm" {
		t.Errorf("expected component=gorm, got %q", v)
	}
}

func TestWarn_LogsAtWarnLevel(t *testing.T) {
	ch := installCapture(t, slog.LevelWarn)
	l := NewGormLogger(0).LogMode(gormlogger.Warn)

	l.Warn(context.Background(), "unindexed query on %s", "users")

	records := ch.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelWarn {
		t.Errorf("expected WARN level, got %v", records[0].Level)
	}
	if v := attrValue(records[0], "component"); v != "gorm" {
		t.Errorf("expected component=gorm, got %q", v)
	}
}

func TestError_LogsAtErrorLevel(t *testing.T) {
	ch := installCapture(t, slog.LevelError)
	l := NewGormLogger(0).LogMode(gormlogger.Error)

	l.Error(context.Background(), "connection failed: %v", errors.New("timeout"))

	records := ch.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelError {
		t.Errorf("expected ERROR level, got %v", records[0].Level)
	}
}

func TestTrace_NormalQuery_LogsAtDebug(t *testing.T) {
	ch := installCapture(t, slog.LevelDebug)
	l := NewGormLogger(0).LogMode(gormlogger.Info)

	begin := time.Now().Add(-5 * time.Millisecond)
	l.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT * FROM `approval_queue` LIMIT 10", 10
	}, nil)

	records := ch.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelDebug {
		t.Errorf("expected DEBUG level, got %v", records[0].Level)
	}
	if records[0].Message != "SQL query" {
		t.Errorf("unexpected message: %q", records[0].Message)
	}
	if v := attrValue(records[0], "sql"); v != "SELECT * FROM `approval_queue` LIMIT 10" {
		t.Errorf("unexpected sql attr: %q", v)
	}
	if v := attrValue(records[0], "component"); v != "gorm" {
		t.Errorf("expected component=gorm, got %q", v)
	}
}

func TestTrace_SlowQuery_LogsAtWarn(t *testing.T) {
	ch := installCapture(t, slog.LevelWarn)
	l := NewGormLogger(50 * time.Millisecond).LogMode(gormlogger.Warn)

	// Simulate a query that took 100ms (well above 50ms threshold)
	begin := time.Now().Add(-100 * time.Millisecond)
	l.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT * FROM `big_table`", 5000
	}, nil)

	records := ch.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelWarn {
		t.Errorf("expected WARN level, got %v", records[0].Level)
	}
	if records[0].Message != "slow SQL query" {
		t.Errorf("unexpected message: %q", records[0].Message)
	}
	if v := attrFloat(records[0], "threshold_ms"); v != 50 {
		t.Errorf("expected threshold_ms=50, got %v", v)
	}
}

func TestTrace_Error_LogsAtError(t *testing.T) {
	ch := installCapture(t, slog.LevelError)
	l := NewGormLogger(0).LogMode(gormlogger.Error)

	begin := time.Now().Add(-2 * time.Millisecond)
	l.Trace(context.Background(), begin, func() (string, int64) {
		return "INSERT INTO `users` VALUES (?)", 0
	}, errors.New("UNIQUE constraint failed: users.email"))

	records := ch.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelError {
		t.Errorf("expected ERROR level, got %v", records[0].Level)
	}
	if records[0].Message != "SQL error" {
		t.Errorf("unexpected message: %q", records[0].Message)
	}
	if v := attrValue(records[0], "error"); v != "UNIQUE constraint failed: users.email" {
		t.Errorf("unexpected error attr: %q", v)
	}
}

func TestTrace_RecordNotFound_DoesNotLogError(t *testing.T) {
	ch := installCapture(t, slog.LevelDebug)
	l := NewGormLogger(0).LogMode(gormlogger.Info)

	begin := time.Now().Add(-1 * time.Millisecond)
	l.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT * FROM `users` WHERE id = ?", 0
	}, gorm.ErrRecordNotFound)

	records := ch.getRecords()
	// ErrRecordNotFound should NOT produce an error-level log.
	// It should fall through to the normal query path (DEBUG).
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != slog.LevelDebug {
		t.Errorf("ErrRecordNotFound should log at DEBUG, got %v", records[0].Level)
	}
	if records[0].Message != "SQL query" {
		t.Errorf("unexpected message: %q", records[0].Message)
	}
}

func TestTrace_SilentLevel_NoOutput(t *testing.T) {
	ch := installCapture(t, slog.LevelDebug)
	l := NewGormLogger(0).LogMode(gormlogger.Silent)

	begin := time.Now().Add(-1 * time.Millisecond)
	l.Trace(context.Background(), begin, func() (string, int64) {
		return "SELECT 1", 1
	}, nil)

	records := ch.getRecords()
	if len(records) != 0 {
		t.Errorf("Silent mode should produce no output, got %d records", len(records))
	}
}

func TestInfo_SuppressedAtWarnLevel(t *testing.T) {
	ch := installCapture(t, slog.LevelDebug)
	l := NewGormLogger(0).LogMode(gormlogger.Warn)

	l.Info(context.Background(), "should be suppressed")

	records := ch.getRecords()
	if len(records) != 0 {
		t.Errorf("Info should be suppressed at Warn level, got %d records", len(records))
	}
}

func TestWarn_SuppressedAtErrorLevel(t *testing.T) {
	ch := installCapture(t, slog.LevelDebug)
	l := NewGormLogger(0).LogMode(gormlogger.Error)

	l.Warn(context.Background(), "should be suppressed")

	records := ch.getRecords()
	if len(records) != 0 {
		t.Errorf("Warn should be suppressed at Error level, got %d records", len(records))
	}
}
