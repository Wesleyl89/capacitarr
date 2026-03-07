package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
)

func TestSettingsService_GetPreferences(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	prefs, err := svc.GetPreferences()
	if err != nil {
		t.Fatalf("GetPreferences returned error: %v", err)
	}

	if prefs.ID != 1 {
		t.Errorf("expected preference ID 1, got %d", prefs.ID)
	}
	if prefs.ExecutionMode != "dry-run" {
		t.Errorf("expected execution mode 'dry-run', got %q", prefs.ExecutionMode)
	}
}

func TestSettingsService_UpdatePreferences(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// Get the current preferences so we have all seeded values
	current, _ := svc.GetPreferences()
	current.PollIntervalSeconds = 600

	updated, err := svc.UpdatePreferences(current)
	if err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}

	if updated.PollIntervalSeconds != 600 {
		t.Errorf("expected poll interval 600, got %d", updated.PollIntervalSeconds)
	}

	// Should publish settings_changed event
	select {
	case evt := <-ch:
		if evt.EventType() != "settings_changed" {
			t.Errorf("expected event type 'settings_changed', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for settings_changed event")
	}
}

func TestSettingsService_UpdatePreferences_ModeChange(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// Get current and change mode
	current, _ := svc.GetPreferences()
	current.ExecutionMode = "approval"

	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}

	// Should publish two events: engine_mode_changed and settings_changed
	receivedTypes := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case evt := <-ch:
			receivedTypes[evt.EventType()] = true
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for events")
		}
	}

	if !receivedTypes["engine_mode_changed"] {
		t.Error("expected engine_mode_changed event")
	}
	if !receivedTypes["settings_changed"] {
		t.Error("expected settings_changed event")
	}
}

func TestSettingsService_UpdateThresholds(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	// Create a disk group to update
	group := db.DiskGroup{
		MountPath:    "/mnt/media",
		TotalBytes:   1000000000,
		UsedBytes:    800000000,
		ThresholdPct: 85.0,
		TargetPct:    75.0,
	}
	if err := database.Create(&group).Error; err != nil {
		t.Fatalf("Failed to create disk group: %v", err)
	}

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	updated, err := svc.UpdateThresholds(group.ID, 90.0, 80.0)
	if err != nil {
		t.Fatalf("UpdateThresholds returned error: %v", err)
	}

	if updated.ThresholdPct != 90.0 {
		t.Errorf("expected threshold 90.0, got %f", updated.ThresholdPct)
	}
	if updated.TargetPct != 80.0 {
		t.Errorf("expected target 80.0, got %f", updated.TargetPct)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "threshold_changed" {
			t.Errorf("expected event type 'threshold_changed', got %q", evt.EventType())
		}
		te, ok := evt.(events.ThresholdChangedEvent)
		if !ok {
			t.Fatal("event is not ThresholdChangedEvent")
		}
		if te.MountPath != "/mnt/media" {
			t.Errorf("expected mount path '/mnt/media', got %q", te.MountPath)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for threshold_changed event")
	}
}

func TestSettingsService_UpdateThresholds_NotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	_, err := svc.UpdateThresholds(99999, 90.0, 80.0)
	if err == nil {
		t.Fatal("expected error for non-existent disk group")
	}
}

func TestSettingsService_ListDiskGroups(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	database.Create(&db.DiskGroup{MountPath: "/mnt/a", TotalBytes: 100, UsedBytes: 50})
	database.Create(&db.DiskGroup{MountPath: "/mnt/b", TotalBytes: 200, UsedBytes: 100})

	groups, err := svc.ListDiskGroups()
	if err != nil {
		t.Fatalf("ListDiskGroups error: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 disk groups, got %d", len(groups))
	}
}

func TestSettingsService_GetDiskGroup(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	database.Create(&db.DiskGroup{MountPath: "/mnt/media", TotalBytes: 1000, UsedBytes: 500})

	group, err := svc.GetDiskGroup(1)
	if err != nil {
		t.Fatalf("GetDiskGroup error: %v", err)
	}
	if group.MountPath != "/mnt/media" {
		t.Errorf("expected mount path '/mnt/media', got %q", group.MountPath)
	}
}

func TestSettingsService_UpsertDiskGroup_Create(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	disk := integrations.DiskSpace{Path: "/mnt/new", TotalBytes: 1000, FreeBytes: 400}
	group, err := svc.UpsertDiskGroup(disk)
	if err != nil {
		t.Fatalf("UpsertDiskGroup error: %v", err)
	}
	if group.MountPath != "/mnt/new" {
		t.Errorf("expected mount path '/mnt/new', got %q", group.MountPath)
	}
	if group.UsedBytes != 600 {
		t.Errorf("expected used bytes 600, got %d", group.UsedBytes)
	}
}

func TestSettingsService_UpsertDiskGroup_Update(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	// Create first
	_, _ = svc.UpsertDiskGroup(integrations.DiskSpace{Path: "/mnt/data", TotalBytes: 1000, FreeBytes: 500})

	// Update
	group, err := svc.UpsertDiskGroup(integrations.DiskSpace{Path: "/mnt/data", TotalBytes: 2000, FreeBytes: 800})
	if err != nil {
		t.Fatalf("UpsertDiskGroup update error: %v", err)
	}
	if group.TotalBytes != 2000 {
		t.Errorf("expected total bytes 2000, got %d", group.TotalBytes)
	}

	// Should still be 1 group
	var count int64
	database.Model(&db.DiskGroup{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 disk group, got %d", count)
	}
}

func TestSettingsService_ListRecentActivities(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	database.Create(&db.ActivityEvent{EventType: "engine_complete", Message: "Done"})
	database.Create(&db.ActivityEvent{EventType: "login", Message: "User logged in"})

	activities, err := svc.ListRecentActivities(1)
	if err != nil {
		t.Fatalf("ListRecentActivities error: %v", err)
	}
	if len(activities) != 1 {
		t.Errorf("expected 1 activity, got %d", len(activities))
	}
}
