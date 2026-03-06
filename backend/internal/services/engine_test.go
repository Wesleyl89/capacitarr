package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
)

func TestEngineService_TriggerRun(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewEngineService(database, bus)

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	result := svc.TriggerRun()
	if result != EngineStatusStarted {
		t.Errorf("expected %q, got %q", EngineStatusStarted, result)
	}

	// Verify event published
	select {
	case evt := <-ch:
		if evt.EventType() != "manual_run_triggered" {
			t.Errorf("expected event type 'manual_run_triggered', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for manual_run_triggered event")
	}

	// Drain the RunNowCh signal
	select {
	case <-svc.RunNowCh:
	default:
		t.Fatal("expected signal on RunNowCh")
	}
}

func TestEngineService_TriggerRun_AlreadyRunning(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewEngineService(database, bus)

	svc.SetRunning(true)
	result := svc.TriggerRun()
	if result != EngineStatusAlreadyRunning {
		t.Errorf("expected %q, got %q", EngineStatusAlreadyRunning, result)
	}
}

func TestEngineService_TriggerRun_ChannelFull(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewEngineService(database, bus)

	// Fill the RunNowCh (buffered with size 1)
	svc.RunNowCh <- struct{}{}

	result := svc.TriggerRun()
	if result != EngineStatusAlreadyRunning {
		t.Errorf("expected %q when channel full, got %q", EngineStatusAlreadyRunning, result)
	}
}

func TestEngineService_SetRunning(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewEngineService(database, bus)

	if svc.IsRunning() {
		t.Error("expected not running initially")
	}

	svc.SetRunning(true)
	if !svc.IsRunning() {
		t.Error("expected running after SetRunning(true)")
	}

	svc.SetRunning(false)
	if svc.IsRunning() {
		t.Error("expected not running after SetRunning(false)")
	}
}

func TestEngineService_SetLastRunStats(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewEngineService(database, bus)

	svc.SetLastRunStats(100, 15, 5)
	stats := svc.GetStats()

	if stats["lastRunEvaluated"] != int64(100) {
		t.Errorf("expected lastRunEvaluated=100, got %v", stats["lastRunEvaluated"])
	}
	if stats["lastRunFlagged"] != int64(15) {
		t.Errorf("expected lastRunFlagged=15, got %v", stats["lastRunFlagged"])
	}
	if stats["protectedCount"] != int64(5) {
		t.Errorf("expected protectedCount=5, got %v", stats["protectedCount"])
	}
}

func TestEngineService_GetStats_WithDBRecord(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewEngineService(database, bus)

	// Seed an engine run stats record
	runStats := db.EngineRunStats{
		RunAt:         time.Now().UTC(),
		Evaluated:     50,
		Flagged:       10,
		Deleted:       3,
		FreedBytes:    5000000000,
		ExecutionMode: "approval",
		DurationMs:    1500,
	}
	if err := database.Create(&runStats).Error; err != nil {
		t.Fatalf("Failed to create engine run stats: %v", err)
	}

	stats := svc.GetStats()

	if stats["executionMode"] != "approval" {
		t.Errorf("expected executionMode 'approval', got %v", stats["executionMode"])
	}
	if stats["lastRunFreedBytes"] != int64(5000000000) {
		t.Errorf("expected lastRunFreedBytes 5000000000, got %v", stats["lastRunFreedBytes"])
	}
}
