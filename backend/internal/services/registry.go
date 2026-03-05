package services

import (
	"gorm.io/gorm"

	"capacitarr/internal/config"
	"capacitarr/internal/events"
)

// Registry holds all service instances. Created once in main.go and passed
// to route registration functions and the poller.
type Registry struct {
	Approval            *ApprovalService
	Deletion            *DeletionService
	AuditLog            *AuditLogService
	Engine              *EngineService
	Settings            *SettingsService
	Integration         *IntegrationService
	Auth                *AuthService
	NotificationChannel *NotificationChannelService
	Data                *DataService
}

// NewRegistry creates a fully wired Registry with all services.
func NewRegistry(database *gorm.DB, bus *events.EventBus, cfg *config.Config) *Registry {
	return &Registry{
		Approval:            NewApprovalService(database, bus),
		Deletion:            NewDeletionService(database, bus),
		AuditLog:            NewAuditLogService(database),
		Engine:              NewEngineService(database, bus),
		Settings:            NewSettingsService(database, bus),
		Integration:         NewIntegrationService(database, bus),
		Auth:                NewAuthService(database, bus, cfg),
		NotificationChannel: NewNotificationChannelService(database, bus),
		Data:                NewDataService(database, bus),
	}
}
