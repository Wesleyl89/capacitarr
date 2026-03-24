package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
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
	if prefs.ExecutionMode != db.ModeDryRun {
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

// mockDeletionQueueClearer implements DeletionQueueClearer for settings tests.
type mockDeletionQueueClearer struct {
	clearCalled int
	clearReturn int
}

func (m *mockDeletionQueueClearer) ClearQueue() int {
	m.clearCalled++
	return m.clearReturn
}

func TestSettingsService_UpdatePreferences_ModeChange_ClearsQueue(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	clearer := &mockDeletionQueueClearer{clearReturn: 5}
	svc.SetDeletionClearer(clearer)

	// Get current prefs (default: dry-run)
	current, _ := svc.GetPreferences()
	current.ExecutionMode = db.ModeApproval

	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}

	if clearer.clearCalled != 1 {
		t.Errorf("expected ClearQueue to be called 1 time, got %d", clearer.clearCalled)
	}
}

func TestSettingsService_UpdatePreferences_DeletionsDisabled_ClearsQueue(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	clearer := &mockDeletionQueueClearer{clearReturn: 3}
	svc.SetDeletionClearer(clearer)

	// First enable deletions
	current, _ := svc.GetPreferences()
	current.DeletionsEnabled = true
	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences (enable) returned error: %v", err)
	}
	clearer.clearCalled = 0 // reset counter

	// Now disable deletions
	current.DeletionsEnabled = false
	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}

	if clearer.clearCalled != 1 {
		t.Errorf("expected ClearQueue to be called 1 time on DeletionsEnabled toggle, got %d", clearer.clearCalled)
	}
}

func TestSettingsService_UpdatePreferences_NoModeChange_NoQueueClear(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	clearer := &mockDeletionQueueClearer{clearReturn: 0}
	svc.SetDeletionClearer(clearer)

	// Update preferences without changing mode
	current, _ := svc.GetPreferences()
	current.PollIntervalSeconds = 600

	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}

	if clearer.clearCalled != 0 {
		t.Errorf("expected ClearQueue NOT to be called when mode unchanged, got %d calls", clearer.clearCalled)
	}
}

func TestSettingsService_UpdatePreferences_DeletionsEnabled_NoQueueClear(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)

	clearer := &mockDeletionQueueClearer{clearReturn: 0}
	svc.SetDeletionClearer(clearer)

	// Start with deletions disabled (default), then enable them
	current, _ := svc.GetPreferences()
	current.DeletionsEnabled = true

	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}

	// Enabling deletions should NOT clear the queue (only disabling does)
	if clearer.clearCalled != 0 {
		t.Errorf("expected ClearQueue NOT to be called when enabling deletions, got %d calls", clearer.clearCalled)
	}
}

func TestSettingsService_UpdatePreferences_NilClearer_NoPanic(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewSettingsService(database, bus)
	// Do NOT set a deletion clearer — should not panic

	current, _ := svc.GetPreferences()
	current.ExecutionMode = db.ModeAuto

	if _, err := svc.UpdatePreferences(current); err != nil {
		t.Fatalf("UpdatePreferences returned error: %v", err)
	}
	// No panic = success
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
