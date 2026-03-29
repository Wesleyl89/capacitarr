package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"

	"gorm.io/gorm"
)

// setupSunsetTest creates a test DB, event bus, sunset service, and a seed disk group.
func setupSunsetTest(t *testing.T) (*gorm.DB, *events.EventBus, *SunsetService) {
	t.Helper()
	database := setupTestDB(t)
	bus := events.NewEventBus()
	t.Cleanup(func() { bus.Close() })
	svc := NewSunsetService(database, bus)
	// Seed FK targets for sunset_queue items
	database.Create(&db.DiskGroup{MountPath: "/data", TotalBytes: 100000, ThresholdPct: 85, TargetPct: 75, Mode: db.ModeSunset})
	database.Create(&db.IntegrationConfig{Type: "sonarr", Name: "Test Sonarr", URL: "http://localhost:8989", APIKey: "test"})
	return database, bus, svc
}

func sunsetDeps(database *gorm.DB, bus *events.EventBus) SunsetDeps {
	return SunsetDeps{Settings: NewSettingsService(database, bus)}
}

func TestQueueSunset(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	item := db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, ExternalID: "1",
		SizeBytes: 5000000000, Score: 0.85, DiskGroupID: 1, Trigger: db.TriggerEngine,
		DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	}

	if err := svc.QueueSunset(item, sunsetDeps(database, bus)); err != nil {
		t.Fatalf("QueueSunset returned error: %v", err)
	}

	items, err := svc.ListAll()
	if err != nil {
		t.Fatalf("ListAll returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}
	if items[0].MediaName != "Firefly" {
		t.Errorf("Expected media name 'Firefly', got %q", items[0].MediaName)
	}
}

func TestBulkQueueSunset(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	items := []db.SunsetQueueItem{
		{MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000, Score: 0.85, DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30)},
		{MediaName: "Serenity", MediaType: "movie", IntegrationID: 1, SizeBytes: 3000000000, Score: 0.70, DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30)},
	}

	created, err := svc.BulkQueueSunset(items, sunsetDeps(database, bus))
	if err != nil {
		t.Fatalf("BulkQueueSunset returned error: %v", err)
	}
	if created != 2 {
		t.Errorf("Expected 2 created, got %d", created)
	}

	all, _ := svc.ListAll()
	if len(all) != 2 {
		t.Errorf("Expected 2 items in queue, got %d", len(all))
	}
}

func TestCancel(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	item := db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	}
	if err := svc.QueueSunset(item, sunsetDeps(database, bus)); err != nil {
		t.Fatalf("QueueSunset setup failed: %v", err)
	}

	items, _ := svc.ListAll()
	if len(items) != 1 {
		t.Fatalf("Expected 1 item before cancel, got %d", len(items))
	}

	if err := svc.Cancel(items[0].ID, sunsetDeps(database, bus)); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	remaining, _ := svc.ListAll()
	if len(remaining) != 0 {
		t.Errorf("Expected 0 items after cancel, got %d", len(remaining))
	}
}

func TestReschedule(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	item := db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	}
	if err := svc.QueueSunset(item, sunsetDeps(database, bus)); err != nil {
		t.Fatalf("QueueSunset setup failed: %v", err)
	}

	items, _ := svc.ListAll()
	if len(items) == 0 {
		t.Fatal("Expected at least 1 item for rescheduling")
	}

	newDate := time.Now().UTC().AddDate(0, 0, 60)
	updated, err := svc.Reschedule(items[0].ID, newDate)
	if err != nil {
		t.Fatalf("Reschedule returned error: %v", err)
	}
	if updated.DeletionDate.Format("2006-01-02") != newDate.Format("2006-01-02") {
		t.Errorf("Expected deletion date %s, got %s", newDate.Format("2006-01-02"), updated.DeletionDate.Format("2006-01-02"))
	}
}

func TestProcessExpired_WithoutDeletion(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	// Create an already-expired item
	database.Create(&db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, -1),
	})

	// Create a future item that should NOT be processed
	database.Create(&db.SunsetQueueItem{
		MediaName: "Serenity", MediaType: "movie", IntegrationID: 1, SizeBytes: 3000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})

	// Without a DeletionService or Registry, expired items should NOT be processed
	// (they remain in the queue for retry when deletion becomes available)
	processed, err := svc.ProcessExpired(sunsetDeps(database, bus))
	if err != nil {
		t.Fatalf("ProcessExpired returned error: %v", err)
	}
	if processed != 0 {
		t.Errorf("Expected 0 processed (no deletion service), got %d", processed)
	}

	// Both items should still be in the queue (expired + future)
	remaining, _ := svc.ListAll()
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining items (no deletions without service), got %d", len(remaining))
	}
}

func TestProcessExpired_WithDeletion(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)
	auditSvc := NewAuditLogService(database)
	deletionSvc := NewDeletionService(bus, auditSvc)

	// Create an already-expired item
	database.Create(&db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, -1),
	})

	// Create a future item
	database.Create(&db.SunsetQueueItem{
		MediaName: "Serenity", MediaType: "movie", IntegrationID: 1, SizeBytes: 3000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})

	// With DeletionService but no Registry, items still can't be processed
	// (Registry is needed to look up the integration's deleter client)
	deps := SunsetDeps{
		Settings: NewSettingsService(database, bus),
		Deletion: deletionSvc,
	}
	processed, err := svc.ProcessExpired(deps)
	if err != nil {
		t.Fatalf("ProcessExpired returned error: %v", err)
	}
	if processed != 0 {
		t.Errorf("Expected 0 processed (no registry), got %d", processed)
	}

	// Future item should still be in queue
	remaining, _ := svc.ListAll()
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining items, got %d", len(remaining))
	}
}

func TestDaysRemaining(t *testing.T) {
	_, _, svc := setupSunsetTest(t)

	tests := []struct {
		name    string
		date    time.Time
		wantMin int
		wantMax int
	}{
		{"30 days future", time.Now().UTC().AddDate(0, 0, 30), 28, 30},
		{"1 day future", time.Now().UTC().AddDate(0, 0, 1), 0, 1},
		{"past date", time.Now().UTC().AddDate(0, 0, -1), 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := db.SunsetQueueItem{DeletionDate: tt.date}
			got := svc.DaysRemaining(item)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("DaysRemaining() = %d, want %d-%d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestListSunsettedKeys(t *testing.T) {
	database, _, svc := setupSunsetTest(t)

	database.Create(&db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})
	database.Create(&db.SunsetQueueItem{
		MediaName: "Serenity", MediaType: "movie", IntegrationID: 1, SizeBytes: 3000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})

	keys, err := svc.ListSunsettedKeys(1)
	if err != nil {
		t.Fatalf("ListSunsettedKeys returned error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(keys))
	}
	if !keys["Firefly|show"] {
		t.Error("Expected key 'Firefly|show' to be present")
	}
	if !keys["Serenity|movie"] {
		t.Error("Expected key 'Serenity|movie' to be present")
	}
}

func TestIsSunsetted(t *testing.T) {
	database, _, svc := setupSunsetTest(t)

	database.Create(&db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})

	if !svc.IsSunsetted("Firefly", "show", 1) {
		t.Error("Expected IsSunsetted=true for queued item")
	}
	if svc.IsSunsetted("Serenity", "movie", 1) {
		t.Error("Expected IsSunsetted=false for non-queued item")
	}
}

func TestCancelAll(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	for i := 0; i < 5; i++ {
		database.Create(&db.SunsetQueueItem{
			MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 1000000,
			DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
		})
	}

	count, err := svc.CancelAll(sunsetDeps(database, bus))
	if err != nil {
		t.Fatalf("CancelAll returned error: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5 cancelled, got %d", count)
	}

	remaining, _ := svc.ListAll()
	if len(remaining) != 0 {
		t.Errorf("Expected 0 remaining, got %d", len(remaining))
	}
}

func TestCancelAllForDiskGroup(t *testing.T) {
	database, bus, svc := setupSunsetTest(t)

	// Create a second disk group
	database.Create(&db.DiskGroup{MountPath: "/data2", TotalBytes: 100000, ThresholdPct: 85, TargetPct: 75, Mode: db.ModeSunset})

	database.Create(&db.SunsetQueueItem{
		MediaName: "Firefly", MediaType: "show", IntegrationID: 1, SizeBytes: 5000000000,
		DiskGroupID: 1, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})
	database.Create(&db.SunsetQueueItem{
		MediaName: "Serenity", MediaType: "movie", IntegrationID: 1, SizeBytes: 3000000000,
		DiskGroupID: 2, Trigger: db.TriggerEngine, DeletionDate: time.Now().UTC().AddDate(0, 0, 30),
	})

	count, err := svc.CancelAllForDiskGroup(1, sunsetDeps(database, bus))
	if err != nil {
		t.Fatalf("CancelAllForDiskGroup returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 cancelled, got %d", count)
	}

	remaining, _ := svc.ListAll()
	if len(remaining) != 1 {
		t.Errorf("Expected 1 remaining, got %d", len(remaining))
	}
	if len(remaining) > 0 && remaining[0].DiskGroupID != 2 {
		t.Errorf("Expected remaining item in disk group 2, got %d", remaining[0].DiskGroupID)
	}
}
