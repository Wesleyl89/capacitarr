package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// NotificationChannelService manages notification channel CRUD.
type NotificationChannelService struct {
	db  *gorm.DB
	bus *events.EventBus
}

// NewNotificationChannelService creates a new NotificationChannelService.
func NewNotificationChannelService(database *gorm.DB, bus *events.EventBus) *NotificationChannelService {
	return &NotificationChannelService{db: database, bus: bus}
}

// Create persists a new notification channel config.
func (s *NotificationChannelService) Create(config db.NotificationConfig) (*db.NotificationConfig, error) {
	if err := s.db.Create(&config).Error; err != nil {
		return nil, fmt.Errorf("failed to create notification channel: %w", err)
	}

	s.bus.Publish(events.NotificationChannelAddedEvent{
		ChannelID:   config.ID,
		ChannelType: config.Type,
		Name:        config.Name,
	})

	return &config, nil
}

// Update modifies an existing notification channel config.
func (s *NotificationChannelService) Update(id uint, config db.NotificationConfig) (*db.NotificationConfig, error) {
	var existing db.NotificationConfig
	if err := s.db.First(&existing, id).Error; err != nil {
		return nil, fmt.Errorf("notification channel not found: %w", err)
	}

	config.ID = id
	if err := s.db.Save(&config).Error; err != nil {
		return nil, fmt.Errorf("failed to update notification channel: %w", err)
	}

	s.bus.Publish(events.NotificationChannelUpdatedEvent{
		ChannelID:   config.ID,
		ChannelType: config.Type,
		Name:        config.Name,
	})

	return &config, nil
}

// Delete removes a notification channel config.
func (s *NotificationChannelService) Delete(id uint) error {
	var config db.NotificationConfig
	if err := s.db.First(&config, id).Error; err != nil {
		return fmt.Errorf("notification channel not found: %w", err)
	}

	if err := s.db.Delete(&config).Error; err != nil {
		return fmt.Errorf("failed to delete notification channel: %w", err)
	}

	s.bus.Publish(events.NotificationChannelRemovedEvent{
		ChannelID:   config.ID,
		ChannelType: config.Type,
		Name:        config.Name,
	})

	return nil
}

// List returns all notification channel configs ordered by id ascending.
func (s *NotificationChannelService) List() ([]db.NotificationConfig, error) {
	var configs []db.NotificationConfig
	if err := s.db.Order("id ASC").Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list notification channels: %w", err)
	}
	return configs, nil
}

// GetByID returns a single notification channel config by primary key.
// Returns ErrNotFound if the record does not exist.
func (s *NotificationChannelService) GetByID(id uint) (*db.NotificationConfig, error) {
	var config db.NotificationConfig
	if err := s.db.First(&config, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get notification channel: %w", err)
	}
	return &config, nil
}

// ListEnabled returns all notification channel configs where enabled = true.
func (s *NotificationChannelService) ListEnabled() ([]db.NotificationConfig, error) {
	var configs []db.NotificationConfig
	if err := s.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list enabled notification channels: %w", err)
	}
	return configs, nil
}

// ListInApp returns in-app notifications ordered by created_at descending, limited to limit rows.
func (s *NotificationChannelService) ListInApp(limit int) ([]db.InAppNotification, error) {
	var notifications []db.InAppNotification
	if err := s.db.Order("created_at DESC").Limit(limit).Find(&notifications).Error; err != nil {
		return nil, fmt.Errorf("failed to list in-app notifications: %w", err)
	}
	return notifications, nil
}

// UnreadCount returns the number of unread in-app notifications.
func (s *NotificationChannelService) UnreadCount() (int64, error) {
	var count int64
	if err := s.db.Model(&db.InAppNotification{}).Where("read = ?", false).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}
	return count, nil
}

// MarkRead marks a single in-app notification as read.
func (s *NotificationChannelService) MarkRead(id uint) error {
	result := s.db.Model(&db.InAppNotification{}).Where("id = ?", id).Update("read", true)
	if result.Error != nil {
		return fmt.Errorf("failed to mark notification as read: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead marks all in-app notifications as read.
func (s *NotificationChannelService) MarkAllRead() error {
	if err := s.db.Model(&db.InAppNotification{}).Where("read = ?", false).Update("read", true).Error; err != nil {
		return fmt.Errorf("failed to mark all notifications as read: %w", err)
	}
	return nil
}

// ClearAllInApp deletes all in-app notifications.
func (s *NotificationChannelService) ClearAllInApp() error {
	if err := s.db.Where("1 = 1").Delete(&db.InAppNotification{}).Error; err != nil {
		return fmt.Errorf("failed to clear in-app notifications: %w", err)
	}
	return nil
}

// PruneOldInApp deletes in-app notifications older than the given cutoff time.
func (s *NotificationChannelService) PruneOldInApp(cutoff time.Time) (int64, error) {
	result := s.db.Where("created_at < ?", cutoff).Delete(&db.InAppNotification{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to prune old in-app notifications: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CreateInApp creates a new in-app notification record.
func (s *NotificationChannelService) CreateInApp(title, message, severity, eventType string) error {
	notif := db.InAppNotification{
		Title:     title,
		Message:   message,
		Severity:  severity,
		EventType: eventType,
	}
	if err := s.db.Create(&notif).Error; err != nil {
		return fmt.Errorf("failed to create in-app notification: %w", err)
	}
	return nil
}
