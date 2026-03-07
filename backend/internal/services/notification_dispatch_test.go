package services

import (
	"sync"
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/notifications"
)

// mockChannelProvider implements ChannelProvider for dispatch tests.
type mockChannelProvider struct {
	configs []db.NotificationConfig
	inApps  []mockInApp
	mu      sync.Mutex
}

type mockInApp struct {
	title, message, severity, eventType string
}

func (m *mockChannelProvider) ListEnabled() ([]db.NotificationConfig, error) {
	return m.configs, nil
}

func (m *mockChannelProvider) GetByID(id uint) (*db.NotificationConfig, error) {
	for _, c := range m.configs {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (m *mockChannelProvider) CreateInApp(title, message, severity, eventType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inApps = append(m.inApps, mockInApp{title, message, severity, eventType})
	return nil
}

// mockSender captures payloads for test assertions.
type mockSender struct {
	mu      sync.Mutex
	digests []notifications.CycleDigest
	alerts  []notifications.Alert
}

func (m *mockSender) SendDigest(_ string, d notifications.CycleDigest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.digests = append(m.digests, d)
	return nil
}

func (m *mockSender) SendAlert(_ string, a notifications.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, a)
	return nil
}

func (m *mockSender) getDigests() []notifications.CycleDigest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]notifications.CycleDigest{}, m.digests...)
}

func (m *mockSender) getAlerts() []notifications.Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]notifications.Alert{}, m.alerts...)
}

// newTestDispatch creates a dispatch service with mock senders for testing.
func newTestDispatch(t *testing.T, channels *mockChannelProvider) (*NotificationDispatchService, *mockSender) {
	t.Helper()
	bus := newTestBus(t)
	svc := NewNotificationDispatchService(bus, channels, nil, "v1.0.0-test")

	// Replace senders with mock
	mock := &mockSender{}
	svc.senders = map[string]notifications.Sender{
		"discord": mock,
		"slack":   mock,
		"inapp":   mock,
	}

	svc.Start()
	t.Cleanup(func() { svc.Stop() })

	return svc, mock
}

func TestNotificationDispatch_TwoGateFlush(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test Discord", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	// Simulate a full engine cycle
	svc.bus.Publish(events.EngineStartEvent{ExecutionMode: "auto"})
	time.Sleep(50 * time.Millisecond)

	svc.bus.Publish(events.EngineCompleteEvent{
		Evaluated:     100,
		Flagged:       3,
		DurationMs:    500,
		ExecutionMode: "auto",
	})
	time.Sleep(50 * time.Millisecond)

	// Gate 2 — no deletion events, just batch complete
	svc.bus.Publish(events.DeletionBatchCompleteEvent{Succeeded: 3, Failed: 0})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].Evaluated != 100 {
		t.Errorf("expected evaluated=100, got %d", digests[0].Evaluated)
	}
	if digests[0].Flagged != 3 {
		t.Errorf("expected flagged=3, got %d", digests[0].Flagged)
	}
}

func TestNotificationDispatch_ReverseGateOrder(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.bus.Publish(events.EngineStartEvent{ExecutionMode: "dry-run"})
	time.Sleep(50 * time.Millisecond)

	// Gate 2 fires first
	svc.bus.Publish(events.DeletionBatchCompleteEvent{Succeeded: 0, Failed: 0})
	time.Sleep(50 * time.Millisecond)

	// Gate 1 fires second — should trigger flush
	svc.bus.Publish(events.EngineCompleteEvent{
		Evaluated:     50,
		Flagged:       0,
		DurationMs:    200,
		ExecutionMode: "dry-run",
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest (reverse gate order), got %d", len(digests))
	}
	if digests[0].ExecutionMode != "dry-run" {
		t.Errorf("expected execution mode 'dry-run', got %q", digests[0].ExecutionMode)
	}
}

func TestNotificationDispatch_Accumulation(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.bus.Publish(events.EngineStartEvent{ExecutionMode: "auto"})
	time.Sleep(50 * time.Millisecond)

	// 3 successful deletions
	for i := 0; i < 3; i++ {
		svc.bus.Publish(events.DeletionSuccessEvent{
			MediaName: "Serenity",
			MediaType: "movie",
			SizeBytes: 1073741824, // 1 GB each
		})
	}
	time.Sleep(50 * time.Millisecond)

	svc.bus.Publish(events.EngineCompleteEvent{
		Evaluated:     200,
		Flagged:       3,
		DurationMs:    1000,
		ExecutionMode: "auto",
	})
	time.Sleep(50 * time.Millisecond)

	svc.bus.Publish(events.DeletionBatchCompleteEvent{Succeeded: 3, Failed: 0})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].Deleted != 3 {
		t.Errorf("expected deleted=3, got %d", digests[0].Deleted)
	}
	if digests[0].FreedBytes != 3*1073741824 {
		t.Errorf("expected freedBytes=%d, got %d", 3*1073741824, digests[0].FreedBytes)
	}
}

func TestNotificationDispatch_ImmediateAlerts(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true,
				OnError: true, OnModeChanged: true, OnServerStarted: true,
				OnThresholdBreach: true, OnUpdateAvailable: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	// EngineErrorEvent → immediate alert
	svc.bus.Publish(events.EngineErrorEvent{Error: "test error"})
	time.Sleep(200 * time.Millisecond)

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert after EngineErrorEvent, got %d", len(alerts))
	}
	if alerts[0].Type != notifications.AlertError {
		t.Errorf("expected alert type 'error', got %q", alerts[0].Type)
	}
}

func TestNotificationDispatch_SubscriptionFiltering(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "No Digest", Enabled: true, OnCycleDigest: false},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.bus.Publish(events.EngineStartEvent{ExecutionMode: "auto"})
	time.Sleep(50 * time.Millisecond)
	svc.bus.Publish(events.EngineCompleteEvent{Evaluated: 10, Flagged: 0, DurationMs: 100, ExecutionMode: "auto"})
	time.Sleep(50 * time.Millisecond)
	svc.bus.Publish(events.DeletionBatchCompleteEvent{Succeeded: 0, Failed: 0})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 0 {
		t.Fatalf("expected 0 digests (channel has OnCycleDigest=false), got %d", len(digests))
	}
}

func TestNotificationDispatch_ModeChangedAlert(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnModeChanged: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.bus.Publish(events.EngineModeChangedEvent{OldMode: "dry-run", NewMode: "auto"})
	time.Sleep(200 * time.Millisecond)

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert for mode change, got %d", len(alerts))
	}
	if alerts[0].Type != notifications.AlertModeChanged {
		t.Errorf("expected alert type 'mode_changed', got %q", alerts[0].Type)
	}
}

func TestNotificationDispatch_ServerStartedAlert(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnServerStarted: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.bus.Publish(events.ServerStartedEvent{Version: "v1.0.0"})
	time.Sleep(200 * time.Millisecond)

	alerts := mock.getAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert for server started, got %d", len(alerts))
	}
	if alerts[0].Type != notifications.AlertServerStarted {
		t.Errorf("expected alert type 'server_started', got %q", alerts[0].Type)
	}
}
