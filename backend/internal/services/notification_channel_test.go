package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
)

func TestNotificationChannelService_Create(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewNotificationChannelService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	config := db.NotificationConfig{
		Type:       "discord",
		Name:       "Dev Alerts",
		WebhookURL: "https://discord.com/api/webhooks/test",
		Enabled:    true,
	}

	result, err := svc.Create(config)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if result.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if result.Name != "Dev Alerts" {
		t.Errorf("expected name 'Dev Alerts', got %q", result.Name)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "notification_channel_added" {
			t.Errorf("expected event type 'notification_channel_added', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification_channel_added event")
	}
}

func TestNotificationChannelService_Update(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewNotificationChannelService(database, bus)

	original := db.NotificationConfig{
		Type: "slack", Name: "Original Slack", WebhookURL: "https://hooks.slack.com/old",
	}
	database.Create(&original)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	updated := db.NotificationConfig{
		Type: "slack", Name: "Updated Slack", WebhookURL: "https://hooks.slack.com/new",
	}

	result, err := svc.Update(original.ID, updated)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if result.Name != "Updated Slack" {
		t.Errorf("expected name 'Updated Slack', got %q", result.Name)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "notification_channel_updated" {
			t.Errorf("expected event type 'notification_channel_updated', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification_channel_updated event")
	}
}

func TestNotificationChannelService_Update_NotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewNotificationChannelService(database, bus)

	_, err := svc.Update(99999, db.NotificationConfig{Name: "ghost"})
	if err == nil {
		t.Fatal("expected error for non-existent channel")
	}
}

func TestNotificationChannelService_Delete(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewNotificationChannelService(database, bus)

	config := db.NotificationConfig{
		Type: "inapp", Name: "In-App Notifications",
	}
	database.Create(&config)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	if err := svc.Delete(config.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Verify deleted from DB
	var count int64
	database.Model(&db.NotificationConfig{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 channels after delete, got %d", count)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "notification_channel_removed" {
			t.Errorf("expected event type 'notification_channel_removed', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification_channel_removed event")
	}
}

func TestNotificationChannelService_Delete_NotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewNotificationChannelService(database, bus)

	err := svc.Delete(99999)
	if err == nil {
		t.Fatal("expected error for non-existent channel")
	}
}
