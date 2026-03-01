package engine

import (
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/integrations"
)

func TestStringMatch(t *testing.T) {
	tests := []struct {
		actual   string
		cond     string
		expected string
		result   bool
	}{
		{"The Matrix", "==", "The Matrix", true},
		{"The Matrix", "==", "the matrix", false}, // strict equal
		{"The Matrix", "!=", "Avatar", true},
		{"The Matrix", "contains", "Matrix", true},
		{"The Matrix", "contains", "Avatar", false},
	}

	for _, tc := range tests {
		t.Run(tc.actual+" "+tc.cond+" "+tc.expected, func(t *testing.T) {
			result := stringMatch(tc.actual, tc.cond, tc.expected)
			if result != tc.result {
				t.Errorf("Expected %v, got %v", tc.result, result)
			}
		})
	}
}

func TestNumberMatch(t *testing.T) {
	tests := []struct {
		actual   float64
		cond     string
		expected float64
		result   bool
	}{
		{5.0, "==", 5.0, true},
		{5.0, "!=", 4.0, true},
		{5.0, ">", 4.0, true},
		{5.0, ">", 5.0, false},
		{5.0, ">=", 5.0, true},
		{5.0, "<", 6.0, true},
		{5.0, "<=", 5.0, true},
	}

	for _, tc := range tests {
		t.Run("NumberMatch", func(t *testing.T) {
			result := numberMatch(tc.actual, tc.cond, tc.expected)
			if result != tc.result {
				t.Errorf("Expected %v, got %v for %v %v %v", tc.result, result, tc.actual, tc.cond, tc.expected)
			}
		})
	}
}

func TestApplyRules(t *testing.T) {
	now := time.Now()

	baseItem := integrations.MediaItem{
		Title:      "The Matrix",
		ShowStatus: "Ended",
		Rating:     8.5,
		AddedAt:    &now,
	}

	tests := []struct {
		name     string
		item     integrations.MediaItem
		rules    []db.ProtectionRule
		isAbs    bool
		modifier float64
	}{
		{
			name:     "No rules",
			item:     baseItem,
			rules:    []db.ProtectionRule{},
			isAbs:    false,
			modifier: 1.0,
		},
		{
			name: "Absolute protect by title",
			item: baseItem,
			rules: []db.ProtectionRule{
				{Type: "protect", Field: "title", Operator: "==", Value: "the matrix", Intensity: "absolute"},
			},
			isAbs:    true,
			modifier: 0.0,
		},
		{
			name: "Strong protect by rating",
			item: baseItem,
			rules: []db.ProtectionRule{
				{Type: "protect", Field: "rating", Operator: ">", Value: "8.0", Intensity: "strong"},
			},
			isAbs:    false,
			modifier: 0.2, // 1.0 * 0.2
		},
		{
			name: "Target absolute by availability",
			item: baseItem,
			rules: []db.ProtectionRule{
				{Type: "target", Field: "availability", Operator: "==", Value: "ended", Intensity: "absolute"},
			},
			isAbs:    false,
			modifier: 100.0,
		},
		{
			name: "Multiple cascading modifiers",
			item: baseItem,
			rules: []db.ProtectionRule{
				{Type: "protect", Field: "rating", Operator: ">", Value: "8.0", Intensity: "strong"},          // * 0.2
				{Type: "protect", Field: "title", Operator: "contains", Value: "matrix", Intensity: "slight"}, // * 0.5
			},
			isAbs:    false,
			modifier: 0.1, // 0.2 * 0.5
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isAbs, modifier, _ := applyRules(tc.item, tc.rules)
			if isAbs != tc.isAbs {
				t.Errorf("Expected absolute protect %v, got %v", tc.isAbs, isAbs)
			}
			// Use small delta for float comparison
			if modifier < tc.modifier-0.01 || modifier > tc.modifier+0.01 {
				t.Errorf("Expected modifier %v, got %v", tc.modifier, modifier)
			}
		})
	}
}
