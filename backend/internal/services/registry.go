package services

import (
	"time"

	"gorm.io/gorm"

	"capacitarr/internal/cache"
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
	Rules               *RulesService
	Metrics             *MetricsService
	Version             *VersionService
	RuleValueCache      *cache.TTLCache
}

// NewRegistry creates a fully wired Registry with all services.
func NewRegistry(database *gorm.DB, bus *events.EventBus, cfg *config.Config) *Registry {
	auditLog := NewAuditLogService(database)
	engineSvc := NewEngineService(database, bus)
	deletionSvc := NewDeletionService(database, bus, auditLog)

	return &Registry{
		DB:                  database,
		Bus:                 bus,
		Cfg:                 cfg,
		Approval:            NewApprovalService(database, bus),
		Deletion:            deletionSvc,
		AuditLog:            auditLog,
		Engine:              engineSvc,
		Settings:            NewSettingsService(database, bus),
		Integration:         NewIntegrationService(database, bus),
		Auth:                NewAuthService(database, bus, cfg),
		NotificationChannel: NewNotificationChannelService(database, bus),
		Data:                NewDataService(database, bus),
		Rules:               NewRulesService(database, bus),
		Metrics:             NewMetricsService(database, engineSvc, deletionSvc),
		RuleValueCache:      cache.New(5 * time.Minute),
	}
}

// InitVersion creates and registers the VersionService. Called by main.go
// after Registry construction, when the application version string is known.
func (r *Registry) InitVersion(appVersion string) {
	r.Version = NewVersionService(r.DB, appVersion, DefaultGitLabReleasesURL)
}
