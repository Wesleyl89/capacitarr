package notifications

import (
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  NotificationTier
	}{
		{"off", TierOff},
		{"critical", TierCritical},
		{"important", TierImportant},
		{"normal", TierNormal},
		{"verbose", TierVerbose},
		{"", TierNormal},        // default
		{"invalid", TierNormal}, // default
	}
	for _, tt := range tests {
		if got := ParseLevel(tt.input); got != tt.want {
			t.Errorf("ParseLevel(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestShouldNotify_TierFiltering(t *testing.T) {
	tests := []struct {
		name      string
		level     NotificationTier
		eventKind string
		want      bool
	}{
		// Critical events should be sent to all levels except off
		{"critical event at verbose", TierVerbose, "error", true},
		{"critical event at normal", TierNormal, "error", true},
		{"critical event at important", TierImportant, "error", true},
		{"critical event at critical", TierCritical, "error", true},
		{"critical event at off", TierOff, "error", false},

		// Threshold breach (critical)
		{"threshold at normal", TierNormal, "threshold_breached", true},
		{"threshold at critical", TierCritical, "threshold_breached", true},

		// Integration down (critical)
		{"integration_down at normal", TierNormal, "integration_down", true},
		{"integration_down at off", TierOff, "integration_down", false},

		// Important events
		{"mode_changed at verbose", TierVerbose, "mode_changed", true},
		{"mode_changed at normal", TierNormal, "mode_changed", true},
		{"mode_changed at important", TierImportant, "mode_changed", true},
		{"mode_changed at critical", TierCritical, "mode_changed", false},

		{"approval at important", TierImportant, "approval_activity", true},
		{"approval at critical", TierCritical, "approval_activity", false},

		// Normal events
		{"digest at verbose", TierVerbose, "cycle_digest", true},
		{"digest at normal", TierNormal, "cycle_digest", true},
		{"digest at important", TierImportant, "cycle_digest", false},
		{"digest at critical", TierCritical, "cycle_digest", false},

		{"update at normal", TierNormal, "update_available", true},
		{"update at important", TierImportant, "update_available", false},

		{"server_started at normal", TierNormal, "server_started", true},
		{"server_started at important", TierImportant, "server_started", false},

		// Verbose events
		{"dry_run at verbose", TierVerbose, "dry_run_digest", true},
		{"dry_run at normal", TierNormal, "dry_run_digest", false},
		{"dry_run at important", TierImportant, "dry_run_digest", false},
		{"dry_run at critical", TierCritical, "dry_run_digest", false},

		{"integration_recovery at verbose", TierVerbose, "integration_recovery", true},
		{"integration_recovery at normal", TierNormal, "integration_recovery", false},

		// Unknown events default to Normal tier
		{"unknown at verbose", TierVerbose, "unknown_event", true},
		{"unknown at normal", TierNormal, "unknown_event", true},
		{"unknown at important", TierImportant, "unknown_event", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldNotify(tt.level, tt.eventKind, nil); got != tt.want {
				t.Errorf("ShouldNotify(%d, %q, nil) = %v, want %v", tt.level, tt.eventKind, got, tt.want)
			}
		})
	}
}

func TestShouldNotify_OverrideTakesPrecedence(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		level    NotificationTier
		event    string
		override *bool
		want     bool
	}{
		// Override true forces notification even when tier would block it
		{"override true at off", TierOff, "error", &trueVal, true},
		{"override true at critical for verbose event", TierCritical, "dry_run_digest", &trueVal, true},

		// Override false blocks notification even when tier would allow it
		{"override false at verbose for critical event", TierVerbose, "error", &falseVal, false},
		{"override false at normal for normal event", TierNormal, "cycle_digest", &falseVal, false},

		// Nil override falls through to tier check
		{"nil override at normal for error", TierNormal, "error", nil, true},
		{"nil override at critical for digest", TierCritical, "cycle_digest", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldNotify(tt.level, tt.event, tt.override); got != tt.want {
				t.Errorf("ShouldNotify(%d, %q, %v) = %v, want %v", tt.level, tt.event, tt.override, got, tt.want)
			}
		})
	}
}

func TestShouldNotify_OffBlocksEverything(t *testing.T) {
	for eventKind := range EventTier {
		if ShouldNotify(TierOff, eventKind, nil) {
			t.Errorf("TierOff should block %q, but ShouldNotify returned true", eventKind)
		}
	}
}

func TestShouldNotify_VerboseAllowsEverything(t *testing.T) {
	for eventKind := range EventTier {
		if !ShouldNotify(TierVerbose, eventKind, nil) {
			t.Errorf("TierVerbose should allow %q, but ShouldNotify returned false", eventKind)
		}
	}
}

func TestEventTier_NoSunsetSpecificEntries(t *testing.T) {
	// Verify no sunset-specific event kinds exist — sunset events map
	// to generic alert types (threshold_breached, error) in the dispatch layer.
	sunsetKeys := []string{
		"sunset_escalation", "sunset_misconfigured", "sunset_activity",
		"sunset_created", "sunset_expired", "sunset_saved",
	}
	for _, key := range sunsetKeys {
		if _, exists := EventTier[key]; exists {
			t.Errorf("EventTier should not contain sunset-specific key %q", key)
		}
	}
}

func TestTierDescription(t *testing.T) {
	// Verify all tiers return non-empty descriptions
	tiers := []NotificationTier{TierOff, TierCritical, TierImportant, TierNormal, TierVerbose}
	for _, tier := range tiers {
		desc := TierDescription(tier)
		if desc == "" {
			t.Errorf("TierDescription(%d) returned empty string", tier)
		}
	}

	// Verify no mode-specific language in descriptions
	for _, tier := range tiers {
		desc := TierDescription(tier)
		for _, word := range []string{"sunset", "auto", "dry-run", "approval"} {
			if contains(desc, word) {
				t.Errorf("TierDescription(%d) contains mode-specific word %q: %s", tier, word, desc)
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
