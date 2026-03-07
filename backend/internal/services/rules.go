package services

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// RuleExportEnvelope wraps exported rules with version metadata.
type RuleExportEnvelope struct {
	Version    int          `json:"version"`
	ExportedAt string       `json:"exportedAt"`
	Rules      []ExportRule `json:"rules"`
}

// ExportRule is a single rule in the portable export format.
type ExportRule struct {
	Field           string  `json:"field"`
	Operator        string  `json:"operator"`
	Value           string  `json:"value"`
	Effect          string  `json:"effect"`
	Enabled         bool    `json:"enabled"`
	IntegrationName *string `json:"integrationName"`
	IntegrationType *string `json:"integrationType"`
}

// ImportRule is a single rule in the incoming import payload.
type ImportRule struct {
	Field           string  `json:"field"`
	Operator        string  `json:"operator"`
	Value           string  `json:"value"`
	Effect          string  `json:"effect"`
	Enabled         bool    `json:"enabled"`
	IntegrationName *string `json:"integrationName"`
	IntegrationType *string `json:"integrationType"`
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
	if !db.ValidEffects[rule.Effect] {
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

// Export returns all custom rules in a portable format with integration names resolved.
func (s *RulesService) Export() (*RuleExportEnvelope, error) {
	rules, err := s.List()
	if err != nil {
		return nil, err
	}

	// Collect all referenced integration IDs
	integrationIDs := make([]uint, 0)
	for _, r := range rules {
		if r.IntegrationID != nil {
			integrationIDs = append(integrationIDs, *r.IntegrationID)
		}
	}

	// Batch-load referenced integrations
	integrationMap := make(map[uint]db.IntegrationConfig)
	if len(integrationIDs) > 0 {
		var configs []db.IntegrationConfig
		if err := s.db.Where("id IN ?", integrationIDs).Find(&configs).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch integrations for export: %w", err)
		}
		for _, ic := range configs {
			integrationMap[ic.ID] = ic
		}
	}

	// Build portable payload
	exported := make([]ExportRule, 0, len(rules))
	for _, r := range rules {
		ep := ExportRule{
			Field:    r.Field,
			Operator: r.Operator,
			Value:    r.Value,
			Effect:   r.Effect,
			Enabled:  r.Enabled,
		}
		if r.IntegrationID != nil {
			if ic, ok := integrationMap[*r.IntegrationID]; ok {
				ep.IntegrationName = &ic.Name
				ep.IntegrationType = &ic.Type
			}
		}
		exported = append(exported, ep)
	}

	envelope := &RuleExportEnvelope{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Rules:      exported,
	}

	s.bus.Publish(events.RulesExportedEvent{Count: len(exported)})

	return envelope, nil
}

// Import creates rules from a portable payload. Integration references are resolved
// using the provided mappings (type:name → integration ID). Returns the number
// of rules imported and a list of unmapped integration keys (if any).
func (s *RulesService) Import(rules []ImportRule, mappings map[string]uint) (int, []string, error) {
	// Resolve integration IDs
	type resolvedRule struct {
		rule          ImportRule
		integrationID *uint
	}
	resolved := make([]resolvedRule, 0, len(rules))
	unmapped := make([]string, 0)

	autoMatchCache := make(map[string]*uint)

	for _, r := range rules {
		// Rule has no integration reference
		if (r.IntegrationName == nil || *r.IntegrationName == "") &&
			(r.IntegrationType == nil || *r.IntegrationType == "") {
			resolved = append(resolved, resolvedRule{rule: r, integrationID: nil})
			continue
		}

		intName := ""
		intType := ""
		if r.IntegrationName != nil {
			intName = *r.IntegrationName
		}
		if r.IntegrationType != nil {
			intType = *r.IntegrationType
		}
		lookupKey := intType + ":" + intName

		// Check explicit mapping first
		if mappings != nil {
			if mappedID, ok := mappings[lookupKey]; ok {
				id := mappedID
				resolved = append(resolved, resolvedRule{rule: r, integrationID: &id})
				continue
			}
		}

		// Auto-match by type and name
		if cachedID, ok := autoMatchCache[lookupKey]; ok {
			if cachedID != nil {
				resolved = append(resolved, resolvedRule{rule: r, integrationID: cachedID})
			} else {
				unmapped = append(unmapped, lookupKey)
			}
			continue
		}

		var ic db.IntegrationConfig
		err := s.db.Where("type = ? AND name = ?", intType, intName).First(&ic).Error
		if err != nil {
			autoMatchCache[lookupKey] = nil
			unmapped = append(unmapped, lookupKey)
			continue
		}
		id := ic.ID
		autoMatchCache[lookupKey] = &id
		resolved = append(resolved, resolvedRule{rule: r, integrationID: &id})
	}

	// De-duplicate unmapped
	unmapped = uniqueStrings(unmapped)

	if len(unmapped) > 0 {
		return 0, unmapped, fmt.Errorf("unmapped integrations")
	}

	// Transactional insert
	tx := s.db.Begin()
	if tx.Error != nil {
		return 0, nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Determine the starting sort_order
	var maxOrder int
	row := tx.Model(&db.CustomRule{}).Select("COALESCE(MAX(sort_order), -1)").Row()
	if err := row.Scan(&maxOrder); err != nil {
		tx.Rollback()
		return 0, nil, fmt.Errorf("failed to determine rule ordering: %w", err)
	}
	nextOrder := maxOrder + 1

	for _, rr := range resolved {
		newRule := db.CustomRule{
			IntegrationID: rr.integrationID,
			Field:         rr.rule.Field,
			Operator:      rr.rule.Operator,
			Value:         rr.rule.Value,
			Effect:        rr.rule.Effect,
			Enabled:       true,
			SortOrder:     nextOrder,
		}
		if err := tx.Create(&newRule).Error; err != nil {
			tx.Rollback()
			return 0, nil, fmt.Errorf("failed to insert imported rule: %w", err)
		}
		// GORM default:true tag ignores false on Create
		if !rr.rule.Enabled {
			if err := tx.Model(&newRule).Update("enabled", false).Error; err != nil {
				tx.Rollback()
				return 0, nil, fmt.Errorf("failed to disable imported rule: %w", err)
			}
		}
		nextOrder++
	}

	if err := tx.Commit().Error; err != nil {
		return 0, nil, fmt.Errorf("failed to commit imported rules: %w", err)
	}

	slog.Info("Imported custom rules", "component", "services", "count", len(resolved))
	s.bus.Publish(events.RulesImportedEvent{Count: len(resolved)})

	return len(resolved), nil, nil
}

// uniqueStrings removes duplicate strings while preserving order.
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool, len(input))
	result := make([]string, 0, len(input))
	for _, str := range input {
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}
	return result
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
