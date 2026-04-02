package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// --------------------------------------------------------------------------
// Backoff calculation
// --------------------------------------------------------------------------

func TestRecoveryState_NextBackoff(t *testing.T) {
	tests := []struct {
		name     string
		failures int
		want     time.Duration
	}{
		{"zero failures", 0, 30 * time.Second},
		{"first failure", 1, 30 * time.Second},
		{"second failure", 2, 60 * time.Second},
		{"third failure", 3, 120 * time.Second},
		{"fourth failure", 4, 240 * time.Second},
		{"fifth failure caps", 5, 5 * time.Minute},
		{"tenth failure caps", 10, 5 * time.Minute},
		{"huge failures caps", 100, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &recoveryState{ConsecutiveFailures: tt.failures}
			got := s.nextBackoff()
			if got != tt.want {
				t.Errorf("nextBackoff() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TrackFailure / TrackRecovery state management
// --------------------------------------------------------------------------

func TestRecoveryService_TrackFailure_NewEntry(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	svc := NewRecoveryService(integrationSvc, bus)

	svc.TrackFailure(1, "sonarr", "My Sonarr", "http://localhost:8989", "key", "connection refused")

	if svc.TrackedCount() != 1 {
		t.Fatalf("expected 1 tracked, got %d", svc.TrackedCount())
	}

	entries := svc.HealthStatus()
	if len(entries) != 1 {
		t.Fatalf("expected 1 health entry, got %d", len(entries))
	}
	if entries[0].IntegrationID != 1 {
		t.Errorf("expected integration ID 1, got %d", entries[0].IntegrationID)
	}
	if entries[0].ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", entries[0].ConsecutiveFailures)
	}
	if entries[0].LastError != "connection refused" {
		t.Errorf("expected error 'connection refused', got %q", entries[0].LastError)
	}
	if !entries[0].Recovering {
		t.Error("expected recovering = true")
	}
}

func TestRecoveryService_TrackFailure_IncrementExisting(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	svc := NewRecoveryService(integrationSvc, bus)

	svc.TrackFailure(1, "sonarr", "My Sonarr", "http://localhost:8989", "key", "error 1")
	svc.TrackFailure(1, "sonarr", "My Sonarr", "http://localhost:8989", "key", "error 2")
	svc.TrackFailure(1, "sonarr", "My Sonarr", "http://localhost:8989", "key", "error 3")

	if svc.TrackedCount() != 1 {
		t.Fatalf("expected 1 tracked (same ID), got %d", svc.TrackedCount())
	}

	entries := svc.HealthStatus()
	if entries[0].ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", entries[0].ConsecutiveFailures)
	}
	if entries[0].LastError != "error 3" {
		t.Errorf("expected last error 'error 3', got %q", entries[0].LastError)
	}
}

func TestRecoveryService_TrackRecovery_RemovesEntry(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	svc := NewRecoveryService(integrationSvc, bus)

	svc.TrackFailure(1, "sonarr", "My Sonarr", "http://localhost:8989", "key", "connection refused")
	if svc.TrackedCount() != 1 {
		t.Fatalf("expected 1 tracked, got %d", svc.TrackedCount())
	}

	svc.TrackRecovery(1)
	if svc.TrackedCount() != 0 {
		t.Fatalf("expected 0 tracked after recovery, got %d", svc.TrackedCount())
	}
}

func TestRecoveryService_TrackRecovery_NoopWhenNotTracked(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	svc := NewRecoveryService(integrationSvc, bus)

	// Should not panic when removing a non-existent entry
	svc.TrackRecovery(999)
	if svc.TrackedCount() != 0 {
		t.Fatalf("expected 0 tracked, got %d", svc.TrackedCount())
	}
}

func TestRecoveryService_MultipleIntegrations(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	svc := NewRecoveryService(integrationSvc, bus)

	svc.TrackFailure(1, "sonarr", "Sonarr", "http://sonarr:8989", "key1", "err1")
	svc.TrackFailure(2, "radarr", "Radarr", "http://radarr:7878", "key2", "err2")
	svc.TrackFailure(3, "plex", "Plex", "http://plex:32400", "key3", "err3")

	if svc.TrackedCount() != 3 {
		t.Fatalf("expected 3 tracked, got %d", svc.TrackedCount())
	}

	// Recover one
	svc.TrackRecovery(2)
	if svc.TrackedCount() != 2 {
		t.Fatalf("expected 2 tracked after recovery, got %d", svc.TrackedCount())
	}

	// Verify remaining entries
	entries := svc.HealthStatus()
	ids := make(map[uint]bool)
	for _, e := range entries {
		ids[e.IntegrationID] = true
	}
	if ids[2] {
		t.Error("integration 2 should have been removed")
	}
	if !ids[1] || !ids[3] {
		t.Error("integrations 1 and 3 should still be tracked")
	}
}

// --------------------------------------------------------------------------
// Seed from DB
// --------------------------------------------------------------------------

func TestRecoveryService_Seed(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)

	// Create integrations — one healthy, one failing
	database.Create(&db.IntegrationConfig{
		Type: "sonarr", Name: "Healthy Sonarr", URL: "http://localhost:8989",
		APIKey: "key1", Enabled: true, LastError: "",
	})
	database.Create(&db.IntegrationConfig{
		Type: "radarr", Name: "Broken Radarr", URL: "http://localhost:7878",
		APIKey: "key2", Enabled: true, LastError: "connection refused",
		ConsecutiveFailures: 3,
	})
	// Use a two-step create for disabled integration: GORM's default:true on the
	// Enabled field causes Create to skip the zero-value false, so we create first
	// then disable via raw update.
	disabledCfg := db.IntegrationConfig{
		Type: "plex", Name: "Disabled Plex", URL: "http://localhost:32400",
		APIKey: "key3", Enabled: true, LastError: "timeout",
	}
	database.Create(&disabledCfg)
	database.Model(&disabledCfg).Update("enabled", false)

	svc := NewRecoveryService(integrationSvc, bus)
	// Calling seed manually (normally called by Start)
	svc.seed()

	// Only the enabled+failing integration should be seeded
	if svc.TrackedCount() != 1 {
		t.Fatalf("expected 1 tracked (only enabled+failing), got %d", svc.TrackedCount())
	}

	entries := svc.HealthStatus()
	if entries[0].Name != "Broken Radarr" {
		t.Errorf("expected 'Broken Radarr', got %q", entries[0].Name)
	}
	if entries[0].ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures from DB seed, got %d", entries[0].ConsecutiveFailures)
	}
}

// --------------------------------------------------------------------------
// UpdateSyncStatus integration with RecoveryTracker
// --------------------------------------------------------------------------

func TestUpdateSyncStatus_TracksFailure(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	recoverySvc := NewRecoveryService(integrationSvc, bus)
	integrationSvc.SetRecoveryTracker(recoverySvc)

	// Create an integration
	cfg := db.IntegrationConfig{
		Type: "sonarr", Name: "Test Sonarr", URL: "http://localhost:8989",
		APIKey: "test-key", Enabled: true,
	}
	database.Create(&cfg)

	// Simulate failure
	err := integrationSvc.UpdateSyncStatus(cfg.ID, nil, "connection refused")
	if err != nil {
		t.Fatalf("UpdateSyncStatus returned error: %v", err)
	}

	// Verify DB was updated
	var updated db.IntegrationConfig
	database.First(&updated, cfg.ID)
	if updated.ConsecutiveFailures != 1 {
		t.Errorf("expected consecutive_failures=1 in DB, got %d", updated.ConsecutiveFailures)
	}
	if updated.LastError != "connection refused" {
		t.Errorf("expected last_error='connection refused', got %q", updated.LastError)
	}

	// Verify recovery tracker was notified
	if recoverySvc.TrackedCount() != 1 {
		t.Fatalf("expected 1 tracked in recovery service, got %d", recoverySvc.TrackedCount())
	}
}

func TestUpdateSyncStatus_TracksRecovery(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	recoverySvc := NewRecoveryService(integrationSvc, bus)
	integrationSvc.SetRecoveryTracker(recoverySvc)

	// Create an integration with existing error
	cfg := db.IntegrationConfig{
		Type: "sonarr", Name: "Test Sonarr", URL: "http://localhost:8989",
		APIKey: "test-key", Enabled: true, LastError: "old error",
		ConsecutiveFailures: 5,
	}
	database.Create(&cfg)

	// Seed the recovery tracker with the failing integration
	recoverySvc.TrackFailure(cfg.ID, cfg.Type, cfg.Name, cfg.URL, cfg.APIKey, cfg.LastError)

	// Simulate success
	now := time.Now()
	err := integrationSvc.UpdateSyncStatus(cfg.ID, &now, "")
	if err != nil {
		t.Fatalf("UpdateSyncStatus returned error: %v", err)
	}

	// Verify DB was updated
	var updated db.IntegrationConfig
	database.First(&updated, cfg.ID)
	if updated.ConsecutiveFailures != 0 {
		t.Errorf("expected consecutive_failures=0 in DB, got %d", updated.ConsecutiveFailures)
	}
	if updated.LastError != "" {
		t.Errorf("expected empty last_error, got %q", updated.LastError)
	}

	// Verify recovery tracker was cleared
	if recoverySvc.TrackedCount() != 0 {
		t.Fatalf("expected 0 tracked after recovery, got %d", recoverySvc.TrackedCount())
	}
}

func TestUpdateSyncStatus_ConsecutiveFailuresIncrement(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	recoverySvc := NewRecoveryService(integrationSvc, bus)
	integrationSvc.SetRecoveryTracker(recoverySvc)

	cfg := db.IntegrationConfig{
		Type: "sonarr", Name: "Test Sonarr", URL: "http://localhost:8989",
		APIKey: "test-key", Enabled: true,
	}
	database.Create(&cfg)

	// Three consecutive failures
	_ = integrationSvc.UpdateSyncStatus(cfg.ID, nil, "error 1")
	_ = integrationSvc.UpdateSyncStatus(cfg.ID, nil, "error 2")
	_ = integrationSvc.UpdateSyncStatus(cfg.ID, nil, "error 3")

	var updated db.IntegrationConfig
	database.First(&updated, cfg.ID)
	if updated.ConsecutiveFailures != 3 {
		t.Errorf("expected consecutive_failures=3 in DB, got %d", updated.ConsecutiveFailures)
	}

	// Recovery resets to 0
	now := time.Now()
	_ = integrationSvc.UpdateSyncStatus(cfg.ID, &now, "")

	database.First(&updated, cfg.ID)
	if updated.ConsecutiveFailures != 0 {
		t.Errorf("expected consecutive_failures=0 after recovery, got %d", updated.ConsecutiveFailures)
	}
}

// --------------------------------------------------------------------------
// IntegrationRecoveryAttemptEvent
// --------------------------------------------------------------------------

func TestIntegrationRecoveryAttemptEvent(t *testing.T) {
	// Success event
	success := events.IntegrationRecoveryAttemptEvent{
		IntegrationID:   1,
		IntegrationType: "sonarr",
		Name:            "My Sonarr",
		Attempt:         3,
		Success:         true,
	}
	if success.EventType() != "integration_recovery_attempt" {
		t.Errorf("expected event type 'integration_recovery_attempt', got %q", success.EventType())
	}
	msg := success.EventMessage()
	if msg == "" {
		t.Error("expected non-empty event message for success")
	}

	// Failure event
	failure := events.IntegrationRecoveryAttemptEvent{
		IntegrationID:    1,
		IntegrationType:  "sonarr",
		Name:             "My Sonarr",
		Attempt:          2,
		Success:          false,
		Error:            "connection refused",
		NextRetrySeconds: 60,
	}
	msg = failure.EventMessage()
	if msg == "" {
		t.Error("expected non-empty event message for failure")
	}
}

// --------------------------------------------------------------------------
// UpdateSyncStatusDirect
// --------------------------------------------------------------------------

func TestUpdateSyncStatusDirect(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)

	cfg := db.IntegrationConfig{
		Type: "sonarr", Name: "Test Sonarr", URL: "http://localhost:8989",
		APIKey: "test-key", Enabled: true, ConsecutiveFailures: 5,
		LastError: "old error",
	}
	database.Create(&cfg)

	now := time.Now()
	err := integrationSvc.UpdateSyncStatusDirect(cfg.ID, &now, "", 0)
	if err != nil {
		t.Fatalf("UpdateSyncStatusDirect returned error: %v", err)
	}

	var updated db.IntegrationConfig
	database.First(&updated, cfg.ID)
	if updated.ConsecutiveFailures != 0 {
		t.Errorf("expected consecutive_failures=0, got %d", updated.ConsecutiveFailures)
	}
	if updated.LastError != "" {
		t.Errorf("expected empty last_error, got %q", updated.LastError)
	}
}

// --------------------------------------------------------------------------
// HealthStatus snapshot
// --------------------------------------------------------------------------

func TestRecoveryService_HealthStatus_Empty(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	integrationSvc := NewIntegrationService(database, bus)
	svc := NewRecoveryService(integrationSvc, bus)

	entries := svc.HealthStatus()
	if len(entries) != 0 {
		t.Fatalf("expected 0 health entries, got %d", len(entries))
	}
}
