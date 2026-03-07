package services

import (
	"errors"
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
)

// errMockDelete is a sentinel error for simulating deletion failures in tests.
var errMockDelete = errors.New("mock delete error")

// mockSettingsReader implements SettingsReader for deletion tests.
type mockSettingsReader struct {
	deletionsEnabled bool
}

func (m *mockSettingsReader) GetPreferences() (db.PreferenceSet, error) {
	return db.PreferenceSet{DeletionsEnabled: m.deletionsEnabled}, nil
}

// mockEngineStatsWriter implements EngineStatsWriter for deletion tests.
type mockEngineStatsWriter struct{}

func (m *mockEngineStatsWriter) IncrementDeletedStats(_ uint, _ int64) error { return nil }

// mockDeletionStatsWriter implements DeletionStatsWriter for deletion tests.
type mockDeletionStatsWriter struct{}

func (m *mockDeletionStatsWriter) IncrementDeletionStats(_ int64) error { return nil }

// mockIntegration implements integrations.Integration for deletion tests.
type mockIntegration struct {
	deleteErr error
}

func (m *mockIntegration) TestConnection() error {
	return nil
}

func (m *mockIntegration) GetDiskSpace() ([]integrations.DiskSpace, error) {
	return nil, nil
}

func (m *mockIntegration) GetRootFolders() ([]string, error) {
	return nil, nil
}

func (m *mockIntegration) GetMediaItems() ([]integrations.MediaItem, error) {
	return nil, nil
}

func (m *mockIntegration) DeleteMediaItem(_ integrations.MediaItem) error {
	return m.deleteErr
}

// drainBatchEvent reads from the bus subscription channel until a
// DeletionBatchCompleteEvent arrives or the timeout expires.
func drainBatchEvent(t *testing.T, ch chan events.Event, timeout time.Duration) *events.DeletionBatchCompleteEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case evt := <-ch:
			if bce, ok := evt.(events.DeletionBatchCompleteEvent); ok {
				return &bce
			}
			// Ignore other events (DeletionSuccessEvent, DeletionDryRunEvent, etc.)
		case <-deadline:
			t.Fatal("timeout waiting for DeletionBatchCompleteEvent")
			return nil
		}
	}
}

func TestDeletionService_SignalBatchSize_Zero(t *testing.T) {
	bus := newTestBus(t)
	auditLog := NewAuditLogService(setupTestDB(t))
	svc := NewDeletionService(bus, auditLog)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// SignalBatchSize(0) should immediately publish DeletionBatchCompleteEvent
	svc.SignalBatchSize(0)

	bce := drainBatchEvent(t, ch, 2*time.Second)
	if bce.Succeeded != 0 {
		t.Errorf("expected Succeeded=0, got %d", bce.Succeeded)
	}
	if bce.Failed != 0 {
		t.Errorf("expected Failed=0, got %d", bce.Failed)
	}
}

func TestDeletionService_BatchTracking_AllSuccess(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	auditLog := NewAuditLogService(database)
	svc := NewDeletionService(bus, auditLog)
	svc.SetDependencies(
		&mockSettingsReader{deletionsEnabled: false}, // dry-run mode
		&mockEngineStatsWriter{},
		&mockDeletionStatsWriter{},
	)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	svc.Start()
	defer svc.Stop()

	// Signal 3 items in this batch
	svc.SignalBatchSize(3)

	// Queue 3 jobs (dry-run mode — deletionsEnabled is false)
	for i := 0; i < 3; i++ {
		job := DeleteJob{
			Client: &mockIntegration{},
			Item: integrations.MediaItem{
				Title:     "Serenity",
				Type:      "movie",
				SizeBytes: 1024 * 1024 * 100,
			},
			Reason: "test",
		}
		if err := svc.QueueDeletion(job); err != nil {
			t.Fatalf("QueueDeletion returned error: %v", err)
		}
	}

	bce := drainBatchEvent(t, ch, 15*time.Second)
	if bce.Succeeded != 3 {
		t.Errorf("expected Succeeded=3, got %d", bce.Succeeded)
	}
	if bce.Failed != 0 {
		t.Errorf("expected Failed=0, got %d", bce.Failed)
	}
}

func TestDeletionService_BatchTracking_MixedSuccessFailure(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	auditLog := NewAuditLogService(database)
	svc := NewDeletionService(bus, auditLog)
	svc.SetDependencies(
		&mockSettingsReader{deletionsEnabled: true}, // actual deletions
		&mockEngineStatsWriter{},
		&mockDeletionStatsWriter{},
	)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	svc.Start()
	defer svc.Stop()

	// Signal 3 items
	svc.SignalBatchSize(3)

	// Queue 2 success + 1 failure
	for i := 0; i < 2; i++ {
		job := DeleteJob{
			Client: &mockIntegration{deleteErr: nil},
			Item: integrations.MediaItem{
				Title:     "Serenity",
				Type:      "movie",
				SizeBytes: 1024 * 1024 * 50,
			},
			Reason: "test",
		}
		if err := svc.QueueDeletion(job); err != nil {
			t.Fatalf("QueueDeletion returned error: %v", err)
		}
	}

	// Queue 1 failure
	failJob := DeleteJob{
		Client: &mockIntegration{deleteErr: errMockDelete},
		Item: integrations.MediaItem{
			Title:     "Firefly",
			Type:      "show",
			SizeBytes: 1024 * 1024 * 200,
		},
		Reason: "test",
	}
	if err := svc.QueueDeletion(failJob); err != nil {
		t.Fatalf("QueueDeletion returned error: %v", err)
	}

	bce := drainBatchEvent(t, ch, 15*time.Second)
	if bce.Succeeded != 2 {
		t.Errorf("expected Succeeded=2, got %d", bce.Succeeded)
	}
	if bce.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", bce.Failed)
	}
}

func TestDeletionService_BatchTracking_CorrectCounts(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	auditLog := NewAuditLogService(database)
	svc := NewDeletionService(bus, auditLog)
	svc.SetDependencies(
		&mockSettingsReader{deletionsEnabled: true},
		&mockEngineStatsWriter{},
		&mockDeletionStatsWriter{},
	)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	svc.Start()
	defer svc.Stop()

	// Signal 5 items: 3 succeed, 2 fail
	svc.SignalBatchSize(5)

	for i := 0; i < 3; i++ {
		_ = svc.QueueDeletion(DeleteJob{
			Client: &mockIntegration{deleteErr: nil},
			Item: integrations.MediaItem{
				Title:     "Serenity",
				Type:      "movie",
				SizeBytes: int64(i+1) * 1024 * 1024 * 10,
			},
			Reason: "test",
		})
	}
	for i := 0; i < 2; i++ {
		_ = svc.QueueDeletion(DeleteJob{
			Client: &mockIntegration{deleteErr: errMockDelete},
			Item: integrations.MediaItem{
				Title:     "Firefly",
				Type:      "show",
				SizeBytes: 1024 * 1024 * 5,
			},
			Reason: "test",
		})
	}

	bce := drainBatchEvent(t, ch, 20*time.Second)
	if bce.Succeeded != 3 {
		t.Errorf("expected Succeeded=3, got %d", bce.Succeeded)
	}
	if bce.Failed != 2 {
		t.Errorf("expected Failed=2, got %d", bce.Failed)
	}
}
