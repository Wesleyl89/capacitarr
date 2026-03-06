package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

func TestIntegrationService_Create(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	config := db.IntegrationConfig{
		Type:   "sonarr",
		Name:   "My Sonarr",
		URL:    "http://localhost:8989",
		APIKey: "abc123",
	}

	result, err := svc.Create(config)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if result.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if result.Name != "My Sonarr" {
		t.Errorf("expected name 'My Sonarr', got %q", result.Name)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "integration_added" {
			t.Errorf("expected event type 'integration_added', got %q", evt.EventType())
		}
		e, ok := evt.(events.IntegrationAddedEvent)
		if !ok {
			t.Fatal("event is not IntegrationAddedEvent")
		}
		if e.Name != "My Sonarr" {
			t.Errorf("expected event name 'My Sonarr', got %q", e.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for integration_added event")
	}
}

func TestIntegrationService_Update(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	// Create first
	original := db.IntegrationConfig{
		Type: "sonarr", Name: "Original", URL: "http://localhost:8989", APIKey: "key1",
	}
	database.Create(&original)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	updated := db.IntegrationConfig{
		Type: "sonarr", Name: "Updated", URL: "http://localhost:8990", APIKey: "key2",
	}

	result, err := svc.Update(original.ID, updated)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if result.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", result.Name)
	}
	if result.URL != "http://localhost:8990" {
		t.Errorf("expected URL 'http://localhost:8990', got %q", result.URL)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "integration_updated" {
			t.Errorf("expected event type 'integration_updated', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for integration_updated event")
	}
}

func TestIntegrationService_Update_NotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	_, err := svc.Update(99999, db.IntegrationConfig{Name: "ghost"})
	if err == nil {
		t.Fatal("expected error for non-existent integration")
	}
}

func TestIntegrationService_Delete(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	config := db.IntegrationConfig{
		Type: "radarr", Name: "My Radarr", URL: "http://localhost:7878", APIKey: "key1",
	}
	database.Create(&config)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	if err := svc.Delete(config.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Verify deleted from DB
	var count int64
	database.Model(&db.IntegrationConfig{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 integrations after delete, got %d", count)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "integration_removed" {
			t.Errorf("expected event type 'integration_removed', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for integration_removed event")
	}
}

func TestIntegrationService_Delete_NotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	err := svc.Delete(99999)
	if err == nil {
		t.Fatal("expected error for non-existent integration")
	}
}

func TestIntegrationService_PublishTestSuccess(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	svc.PublishTestSuccess("sonarr", "My Sonarr", "http://localhost:8989")

	select {
	case evt := <-ch:
		if evt.EventType() != "integration_test" {
			t.Errorf("expected event type 'integration_test', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for integration_test event")
	}
}

func TestIntegrationService_PublishTestFailure(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewIntegrationService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	svc.PublishTestFailure("sonarr", "My Sonarr", "http://localhost:8989", "connection refused")

	select {
	case evt := <-ch:
		if evt.EventType() != "integration_test_failed" {
			t.Errorf("expected event type 'integration_test_failed', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for integration_test_failed event")
	}
}
