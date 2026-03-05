package services

import (
	"fmt"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/logger"
)

// SettingsService manages application preferences and disk group thresholds.
type SettingsService struct {
	db  *gorm.DB
	bus *events.EventBus
}

// NewSettingsService creates a new SettingsService.
func NewSettingsService(database *gorm.DB, bus *events.EventBus) *SettingsService {
	return &SettingsService{db: database, bus: bus}
}

// GetPreferences returns the current preferences (singleton row).
func (s *SettingsService) GetPreferences() (db.PreferenceSet, error) {
	var pref db.PreferenceSet
	if err := s.db.FirstOrCreate(&pref, db.PreferenceSet{ID: 1}).Error; err != nil {
		return pref, fmt.Errorf("failed to fetch preferences: %w", err)
	}
	return pref, nil
}

// UpdatePreferences saves preference changes and publishes relevant events.
func (s *SettingsService) UpdatePreferences(payload db.PreferenceSet) (db.PreferenceSet, error) {
	payload.ID = 1

	// Snapshot current for change detection
	var oldPrefs db.PreferenceSet
	s.db.FirstOrCreate(&oldPrefs, db.PreferenceSet{ID: 1})

	if err := s.db.Save(&payload).Error; err != nil {
		return payload, fmt.Errorf("failed to save preferences: %w", err)
	}

	// Apply dynamic log level
	logger.SetLevel(payload.LogLevel)

	// Detect execution mode change
	if oldPrefs.ExecutionMode != payload.ExecutionMode {
		s.bus.Publish(events.EngineModeChangedEvent{
			OldMode: oldPrefs.ExecutionMode,
			NewMode: payload.ExecutionMode,
		})
	}

	s.bus.Publish(events.SettingsChangedEvent{})

	return payload, nil
}

// UpdateThresholds updates the threshold and target percentages for a disk group.
func (s *SettingsService) UpdateThresholds(groupID uint, threshold, target float64) error {
	var group db.DiskGroup
	if err := s.db.First(&group, groupID).Error; err != nil {
		return fmt.Errorf("disk group not found: %w", err)
	}

	if err := s.db.Model(&group).Updates(map[string]interface{}{
		"threshold_pct": threshold,
		"target_pct":    target,
	}).Error; err != nil {
		return fmt.Errorf("failed to update thresholds: %w", err)
	}

	s.bus.Publish(events.ThresholdChangedEvent{
		MountPath:    group.MountPath,
		ThresholdPct: threshold,
		TargetPct:    target,
	})

	return nil
}
