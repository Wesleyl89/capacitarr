package services

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// validEffects enumerates the allowed values for CustomRule.Effect.
var validEffects = map[string]bool{
	"always_keep": true, "prefer_keep": true, "lean_keep": true,
	"lean_remove": true, "prefer_remove": true, "always_remove": true,
}

// RulesService manages custom rule CRUD and reordering.
type RulesService struct {
	db  *gorm.DB
	bus *events.EventBus
}

// NewRulesService creates a new RulesService.
func NewRulesService(database *gorm.DB, bus *events.EventBus) *RulesService {
	return &RulesService{db: database, bus: bus}
}

// List returns all custom rules ordered by sort_order ASC, id ASC.
func (s *RulesService) List() ([]db.CustomRule, error) {
	rules := make([]db.CustomRule, 0)
	if err := s.db.Order("sort_order ASC, id ASC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch custom rules: %w", err)
	}
	return rules, nil
}

// Create validates and persists a new custom rule.
func (s *RulesService) Create(rule db.CustomRule) (*db.CustomRule, error) {
	// Validate required fields
	if rule.Field == "" || rule.Operator == "" || rule.Value == "" {
		return nil, fmt.Errorf("field, operator, and value are required")
	}

	// Validate effect
	if rule.Effect == "" {
		return nil, fmt.Errorf("effect field is required")
	}
	if !validEffects[rule.Effect] {
		return nil, fmt.Errorf("effect must be one of: always_keep, prefer_keep, lean_keep, lean_remove, prefer_remove, always_remove")
	}

	// Ensure new rules are enabled by default
	rule.Enabled = true

	if err := s.db.Create(&rule).Error; err != nil {
		slog.Error("Failed to create custom rule", "component", "services", "error", err)
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	s.bus.Publish(events.RuleCreatedEvent{
		RuleID: rule.ID,
		Field:  rule.Field,
		Effect: rule.Effect,
	})

	return &rule, nil
}

// Update modifies an existing custom rule identified by id.
func (s *RulesService) Update(id uint, rule db.CustomRule) (*db.CustomRule, error) {
	var existing db.CustomRule
	if err := s.db.First(&existing, id).Error; err != nil {
		return nil, fmt.Errorf("rule not found: %w", err)
	}

	// Preserve the ID from the existing record
	rule.ID = existing.ID
	if err := s.db.Save(&rule).Error; err != nil {
		slog.Error("Failed to update custom rule", "component", "services", "id", id, "error", err)
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}

	s.bus.Publish(events.RuleUpdatedEvent{
		RuleID: rule.ID,
		Field:  rule.Field,
		Effect: rule.Effect,
	})

	return &rule, nil
}

// Delete removes a custom rule identified by id.
func (s *RulesService) Delete(id uint) error {
	var existing db.CustomRule
	if err := s.db.First(&existing, id).Error; err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}

	if err := s.db.Delete(&existing).Error; err != nil {
		slog.Error("Failed to delete custom rule", "component", "services", "id", id, "error", err)
		return fmt.Errorf("failed to delete rule: %w", err)
	}

	s.bus.Publish(events.RuleDeletedEvent{
		RuleID: existing.ID,
		Field:  existing.Field,
	})

	return nil
}

// Reorder updates the sort_order for each rule ID in the provided slice.
// The position in the slice determines the new sort_order value.
func (s *RulesService) Reorder(ids []uint) error {
	tx := s.db.Begin()
	for idx, ruleID := range ids {
		if err := tx.Model(&db.CustomRule{}).Where("id = ?", ruleID).Update("sort_order", idx).Error; err != nil {
			tx.Rollback()
			slog.Error("Failed to update rule sort order", "component", "services", "ruleId", ruleID, "error", err)
			return fmt.Errorf("failed to reorder rules: %w", err)
		}
	}
	return tx.Commit().Error
}
