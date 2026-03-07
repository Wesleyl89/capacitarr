package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("record not found")

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

// List returns all integration configs ordered by created_at ascending.
func (s *IntegrationService) List() ([]db.IntegrationConfig, error) {
	var configs []db.IntegrationConfig
	if err := s.db.Order("created_at asc").Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list integrations: %w", err)
	}
	return configs, nil
}

// GetByID returns a single integration config by primary key.
// Returns ErrNotFound if the record does not exist.
func (s *IntegrationService) GetByID(id uint) (*db.IntegrationConfig, error) {
	var config db.IntegrationConfig
	if err := s.db.First(&config, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get integration: %w", err)
	}
	return &config, nil
}

// ListEnabled returns all integration configs where enabled = true.
func (s *IntegrationService) ListEnabled() ([]db.IntegrationConfig, error) {
	var configs []db.IntegrationConfig
	if err := s.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list enabled integrations: %w", err)
	}
	return configs, nil
}

// UpdateSyncStatus updates the last_sync and last_error fields on an integration config.
func (s *IntegrationService) UpdateSyncStatus(id uint, lastSync *time.Time, lastError string) error {
	result := s.db.Model(&db.IntegrationConfig{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_sync":  lastSync,
		"last_error": lastError,
	})
	if result.Error != nil {
		return fmt.Errorf("failed to update sync status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SyncResult holds the outcome of syncing a single integration.
type SyncResult struct {
	ID         uint                     `json:"id"`
	Name       string                   `json:"name"`
	Type       string                   `json:"type"`
	Status     string                   `json:"status"`
	Error      string                   `json:"error,omitempty"`
	DiskError  string                   `json:"diskError,omitempty"`
	DiskSpace  []integrations.DiskSpace `json:"diskSpace,omitempty"`
	MediaCount int                      `json:"mediaCount,omitempty"`
	MediaError string                   `json:"mediaError,omitempty"`
}

// SyncAll fetches data from all enabled integrations: tests connections,
// retrieves disk space (upserting DiskGroups), and counts media items.
func (s *IntegrationService) SyncAll() ([]SyncResult, error) {
	configs, err := s.ListEnabled()
	if err != nil {
		return nil, err
	}

	results := make([]SyncResult, 0, len(configs))
	for _, cfg := range configs {
		client := integrations.NewClient(cfg.Type, cfg.URL, cfg.APIKey)
		if client == nil {
			continue
		}

		result := SyncResult{
			ID:   cfg.ID,
			Name: cfg.Name,
			Type: cfg.Type,
		}

		// Test connection
		if connErr := client.TestConnection(); connErr != nil {
			result.Status = "error"
			result.Error = connErr.Error()
			results = append(results, result)
			continue
		}

		// Get disk space
		disks, diskErr := client.GetDiskSpace()
		if diskErr != nil {
			result.DiskError = diskErr.Error()
		} else {
			result.DiskSpace = disks
			for _, d := range disks {
				s.upsertDiskGroup(d)
			}
		}

		// Get media items count
		items, mediaErr := client.GetMediaItems()
		if mediaErr != nil {
			result.MediaError = mediaErr.Error()
		} else {
			result.MediaCount = len(items)
		}

		result.Status = "ok"
		results = append(results, result)
	}

	return results, nil
}

// upsertDiskGroup creates or updates a DiskGroup from discovered disk space.
func (s *IntegrationService) upsertDiskGroup(disk integrations.DiskSpace) {
	var group db.DiskGroup
	result := s.db.Where("mount_path = ?", disk.Path).First(&group)

	usedBytes := disk.TotalBytes - disk.FreeBytes

	if result.Error != nil {
		// Create new disk group
		group = db.DiskGroup{
			MountPath:  disk.Path,
			TotalBytes: disk.TotalBytes,
			UsedBytes:  usedBytes,
		}
		s.db.Create(&group)
	} else {
		// Update existing
		s.db.Model(&group).Updates(map[string]interface{}{
			"total_bytes": disk.TotalBytes,
			"used_bytes":  usedBytes,
		})
	}
}
