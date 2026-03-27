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

// mockSender captures payloads for test assertions.
type mockSender struct {
	mu      sync.Mutex
	digests []notifications.CycleDigest
	alerts  []notifications.Alert
}

func (m *mockSender) SendDigest(_ notifications.SenderConfig, d notifications.CycleDigest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.digests = append(m.digests, d)
	return nil
}

func (m *mockSender) SendAlert(_ notifications.SenderConfig, a notifications.Alert) error {
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

// newTestDispatch creates a dispatch service with mock senders for
// external channels (discord/apprise). Returns the service and the mock sender.
func newTestDispatch(t *testing.T, channels *mockChannelProvider) (*NotificationDispatchService, *mockSender) {
	t.Helper()
	bus := newTestBus(t)
	svc := NewNotificationDispatchService(bus, channels, nil, "v1.0.0-test")

	// Replace senders with a mock so assertions can inspect payloads.
	externalMock := &mockSender{}
	svc.senders = map[string]notifications.Sender{
		"discord": externalMock,
		"apprise": externalMock,
	}

	svc.Start()
	t.Cleanup(func() { svc.Stop() })

	return svc, externalMock
}

func TestNotificationDispatch_FlushCycleDigest(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test Discord", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	// FlushCycleDigest is called directly by the poller — no event sequence needed.
	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeAuto,
		Evaluated:     100,
		Candidates:    3,
		Deleted:       3,
		FreedBytes:    3 * 1073741824,
		DurationMs:    500,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].Evaluated != 100 {
		t.Errorf("expected evaluated=100, got %d", digests[0].Evaluated)
	}
	if digests[0].Candidates != 3 {
		t.Errorf("expected candidates=3, got %d", digests[0].Candidates)
	}
	if digests[0].Deleted != 3 {
		t.Errorf("expected deleted=3, got %d", digests[0].Deleted)
	}
	if digests[0].FreedBytes != 3*1073741824 {
		t.Errorf("expected freedBytes=%d, got %d", 3*1073741824, digests[0].FreedBytes)
	}
	// FlushCycleDigest should set the version from the service
	if digests[0].Version != "v1.0.0-test" {
		t.Errorf("expected version='v1.0.0-test', got %q", digests[0].Version)
	}
}

func TestNotificationDispatch_FlushCycleDigest_DryRun(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnCycleDigest: true, OnDryRunDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeDryRun,
		Evaluated:     50,
		Candidates:    5,
		FreedBytes:    1073741824,
		DurationMs:    200,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].ExecutionMode != db.ModeDryRun {
		t.Errorf("expected execution mode 'dry-run', got %q", digests[0].ExecutionMode)
	}
}

func TestNotificationDispatch_FlushCycleDigest_CollectionGroups(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode:      db.ModeAuto,
		Evaluated:          200,
		Candidates:         10,
		Deleted:            10,
		FreedBytes:         5 * 1073741824,
		DurationMs:         1000,
		CollectionsDeleted: 2,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].CollectionsDeleted != 2 {
		t.Errorf("expected collectionsDeleted=2, got %d", digests[0].CollectionsDeleted)
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

	// FlushCycleDigest respects OnCycleDigest=false
	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeAuto,
		Evaluated:     10,
		Candidates:    0,
		DurationMs:    100,
	})
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

	svc.bus.Publish(events.EngineModeChangedEvent{OldMode: db.ModeDryRun, NewMode: db.ModeAuto})
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

func TestNotificationDispatch_ApprovalActivityFiltering(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "No Approval", Enabled: true, OnApprovalActivity: false},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.bus.Publish(events.ApprovalApprovedEvent{MediaName: "Serenity", MediaType: "movie"})
	time.Sleep(200 * time.Millisecond)

	alerts := mock.getAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts (channel has OnApprovalActivity=false), got %d", len(alerts))
	}
}

func TestNotificationDispatch_ApprovalModeFreedBytes(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnCycleDigest: true, OnApprovalActivity: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	// In approval mode, FreedBytes comes from the poller's counters.
	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeApproval,
		Evaluated:     2232,
		Candidates:    80,
		FreedBytes:    5368709120, // ~5 GB potential savings
		DurationMs:    11900,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].FreedBytes != 5368709120 {
		t.Errorf("expected freedBytes=5368709120, got %d", digests[0].FreedBytes)
	}
	if digests[0].ExecutionMode != db.ModeApproval {
		t.Errorf("expected execution mode 'approval', got %q", digests[0].ExecutionMode)
	}
	if digests[0].Candidates != 80 {
		t.Errorf("expected candidates=80, got %d", digests[0].Candidates)
	}
}

func TestNotificationDispatch_ApprovalModeDigestSuppressed(t *testing.T) { //nolint:dupl // test structure intentionally similar
	// When OnApprovalActivity=false, approval-mode cycle digests should be
	// suppressed — users who turn off "Approval Activity" expect ALL
	// approval-related notifications to stop, including the engine cycle
	// digest that says "Items Queued for Approval".
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "No Approval Digest", Enabled: true,
				OnCycleDigest: true, OnApprovalActivity: false},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeApproval,
		Evaluated:     100,
		Candidates:    5,
		FreedBytes:    1073741824,
		DurationMs:    500,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 0 {
		t.Fatalf("expected 0 digests (OnApprovalActivity=false suppresses approval-mode digests), got %d", len(digests))
	}
}

func TestNotificationDispatch_DryRunDigestSuppressed(t *testing.T) { //nolint:dupl // test structure intentionally similar
	// When OnDryRunDigest=false, dry-run cycle digests should be suppressed —
	// users who turn off "Include Dry-Run" expect the periodic "would delete
	// N items" summaries to stop, while still receiving auto-mode digests.
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "No DryRun Digest", Enabled: true,
				OnCycleDigest: true, OnDryRunDigest: false},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeDryRun,
		Evaluated:     100,
		Candidates:    5,
		FreedBytes:    1073741824,
		DurationMs:    500,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 0 {
		t.Fatalf("expected 0 digests (OnDryRunDigest=false suppresses dry-run digests), got %d", len(digests))
	}
}

func TestNotificationDispatch_NonDryRunDigestNotAffected(t *testing.T) {
	// When OnDryRunDigest=false, auto-mode cycle digests should still
	// be delivered normally — only dry-run digests are suppressed.
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true,
				OnCycleDigest: true, OnDryRunDigest: false},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeAuto,
		Evaluated:     50,
		Candidates:    2,
		Deleted:       2,
		DurationMs:    300,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest (auto mode unaffected by OnDryRunDigest=false), got %d", len(digests))
	}
	if digests[0].ExecutionMode != db.ModeAuto {
		t.Errorf("expected execution mode 'auto', got %q", digests[0].ExecutionMode)
	}
}

func TestNotificationDispatch_NonApprovalDigestNotAffected(t *testing.T) {
	// When OnApprovalActivity=false, non-approval-mode digests (auto)
	// should still be delivered normally.
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true,
				OnCycleDigest: true, OnApprovalActivity: false},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeAuto,
		Evaluated:     50,
		Candidates:    2,
		Deleted:       2,
		DurationMs:    300,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest (auto mode unaffected by OnApprovalActivity=false), got %d", len(digests))
	}
	if digests[0].ExecutionMode != db.ModeAuto {
		t.Errorf("expected execution mode 'auto', got %q", digests[0].ExecutionMode)
	}
}

func TestNotificationDispatch_AppriseChannel(t *testing.T) {
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "apprise", Name: "Apprise Server", WebhookURL: "http://apprise:8000/api/notify/mykey/",
				AppriseTags: "admin", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeAuto,
		Evaluated:     50,
		Candidates:    2,
		Deleted:       2,
		DurationMs:    300,
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest from apprise channel, got %d", len(digests))
	}
}

func TestNotificationDispatch_VersionPopulated(t *testing.T) {
	// Verify FlushCycleDigest populates the version from the service,
	// overriding any value in the input digest.
	channels := &mockChannelProvider{
		configs: []db.NotificationConfig{
			{ID: 1, Type: "discord", Name: "Test", Enabled: true, OnCycleDigest: true},
		},
	}

	svc, mock := newTestDispatch(t, channels)

	// Pass a digest with a different version — should be overridden
	svc.FlushCycleDigest(notifications.CycleDigest{
		ExecutionMode: db.ModeAuto,
		Evaluated:     10,
		Version:       "should-be-overridden",
	})
	time.Sleep(200 * time.Millisecond)

	digests := mock.getDigests()
	if len(digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(digests))
	}
	if digests[0].Version != "v1.0.0-test" {
		t.Errorf("expected version='v1.0.0-test', got %q", digests[0].Version)
	}
}
