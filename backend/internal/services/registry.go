package services

import (
	"gorm.io/gorm"

	"capacitarr/internal/config"
	"capacitarr/internal/events"
)

// Registry holds all service instances and shared dependencies. Created once
// in main.go and passed to route registration functions and the poller.
//
// DB and Bus are exposed for route handlers that need raw read access (e.g.,
// listing items, metrics queries). Write operations and business logic should
// always go through the appropriate service.
type Registry struct {
	DB  *gorm.DB
	Bus *events.EventBus
	Cfg *config.Config

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
	auditLog := NewAuditLogService(database)
	return &Registry{
		DB:                  database,
		Bus:                 bus,
		Cfg:                 cfg,
		Approval:            NewApprovalService(database, bus),
		Deletion:            NewDeletionService(database, bus, auditLog),
		AuditLog:            auditLog,
		Engine:              NewEngineService(database, bus),
		Settings:            NewSettingsService(database, bus),
		Integration:         NewIntegrationService(database, bus),
		Auth:                NewAuthService(database, bus, cfg),
		NotificationChannel: NewNotificationChannelService(database, bus),
		Data:                NewDataService(database, bus),
	}
}
