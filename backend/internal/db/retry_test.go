package db

import (
	"errors"
	"testing"
)

func TestIsSQLiteBusy(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "database is locked",
			err:  errors.New("sqlite3: database is locked"),
			want: true,
		},
		{
			name: "SQLITE_BUSY",
			err:  errors.New("SQLITE_BUSY (5)"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("UNIQUE constraint failed"),
			want: false,
		},
		{
			name: "wrapped database is locked",
			err:  errors.New("failed to insert: sqlite3: database is locked"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSQLiteBusy(tt.err)
			if got != tt.want {
				t.Errorf("isSQLiteBusy(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name    string
		dbPath  string
		wantHas []string
		wantNot []string
	}{
		{
			name:    "in-memory database unchanged",
			dbPath:  ":memory:",
			wantHas: []string{":memory:"},
			wantNot: []string{"file:", "_pragma"},
		},
		{
			name:    "file URI unchanged",
			dbPath:  "file:test.db?_pragma=busy_timeout(1000)",
			wantHas: []string{"file:test.db"},
			wantNot: nil,
		},
		{
			name:   "bare path converted to file URI with WAL and busy_timeout",
			dbPath: "/config/capacitarr.db",
			wantHas: []string{
				"file:/config/capacitarr.db",
				"journal_mode",
				"wal",
				"busy_timeout",
				"5000",
				"_txlock=immediate",
			},
			wantNot: nil,
		},
		{
			name:   "relative path converted",
			dbPath: "test.db",
			wantHas: []string{
				"file:test.db",
				"journal_mode",
				"wal",
			},
			wantNot: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDSN(tt.dbPath)
			for _, want := range tt.wantHas {
				if !containsStr(got, want) {
					t.Errorf("buildDSN(%q) = %q, want to contain %q", tt.dbPath, got, want)
				}
			}
			for _, notWant := range tt.wantNot {
				if containsStr(got, notWant) {
					t.Errorf("buildDSN(%q) = %q, should not contain %q", tt.dbPath, got, notWant)
				}
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
