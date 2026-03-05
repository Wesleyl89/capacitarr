package services

import (
	"fmt"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// IntegrationService manages integration CRUD and connection testing.
type IntegrationService struct {
	db  *gorm.DB
	bus *events.EventBus
}

// NewIntegrationService creates a new IntegrationService.
func NewIntegrationService(database *gorm.DB, bus *events.EventBus) *IntegrationService {
	return &IntegrationService{db: database, bus: bus}
}

// Create persists a new integration config.
func (s *IntegrationService) Create(config db.IntegrationConfig) (*db.IntegrationConfig, error) {
	if err := s.db.Create(&config).Error; err != nil {
		return nil, fmt.Errorf("failed to create integration: %w", err)
	}

	s.bus.Publish(events.IntegrationAddedEvent{
		IntegrationID:   config.ID,
		IntegrationType: config.Type,
		Name:            config.Name,
	})

	return &config, nil
}

// Update modifies an existing integration config.
func (s *IntegrationService) Update(id uint, config db.IntegrationConfig) (*db.IntegrationConfig, error) {
	var existing db.IntegrationConfig
	if err := s.db.First(&existing, id).Error; err != nil {
		return nil, fmt.Errorf("integration not found: %w", err)
	}

	config.ID = id
	if err := s.db.Save(&config).Error; err != nil {
		return nil, fmt.Errorf("failed to update integration: %w", err)
	}

	s.bus.Publish(events.IntegrationUpdatedEvent{
		IntegrationID:   config.ID,
		IntegrationType: config.Type,
		Name:            config.Name,
	})

	return &config, nil
}

// Delete removes an integration config.
func (s *IntegrationService) Delete(id uint) error {
	var config db.IntegrationConfig
	if err := s.db.First(&config, id).Error; err != nil {
		return fmt.Errorf("integration not found: %w", err)
	}

	if err := s.db.Delete(&config).Error; err != nil {
		return fmt.Errorf("failed to delete integration: %w", err)
	}

	s.bus.Publish(events.IntegrationRemovedEvent{
		IntegrationID:   config.ID,
		IntegrationType: config.Type,
		Name:            config.Name,
	})

	return nil
}

// PublishTestSuccess publishes a successful connection test event.
func (s *IntegrationService) PublishTestSuccess(intType, name, url string) {
	s.bus.Publish(events.IntegrationTestEvent{
		IntegrationType: intType,
		Name:            name,
		URL:             url,
	})
}

// PublishTestFailure publishes a failed connection test event.
func (s *IntegrationService) PublishTestFailure(intType, name, url, errMsg string) {
	s.bus.Publish(events.IntegrationTestFailedEvent{
		IntegrationType: intType,
		Name:            name,
		URL:             url,
		Error:           errMsg,
	})
}
