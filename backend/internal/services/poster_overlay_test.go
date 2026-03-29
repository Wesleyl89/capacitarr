package services

import (
	"testing"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/poster"
)

func setupPosterOverlayTest(t *testing.T) (*PosterOverlayService, *events.EventBus) {
	t.Helper()
	database := setupTestDB(t)
	bus := events.NewEventBus()
	t.Cleanup(func() { bus.Close() })

	cacheDir := t.TempDir()
	svc, err := NewPosterOverlayService(database, bus, cacheDir)
	if err != nil {
		t.Fatalf("NewPosterOverlayService failed: %v", err)
	}
	return svc, bus
}

func TestPosterOverlayService_RestoreAll_EmptyQueue(t *testing.T) {
	svc, bus := setupPosterOverlayTest(t)
	sunset := NewSunsetService(setupTestDB(t), bus)

	restored, err := svc.RestoreAll(sunset, PosterDeps{})
	if err != nil {
		t.Fatalf("RestoreAll failed: %v", err)
	}
	if restored != 0 {
		t.Errorf("Expected 0 restored, got %d", restored)
	}
}

func TestPosterOverlayService_ValidateCache_NoActiveOverlays(t *testing.T) {
	svc, _ := setupPosterOverlayTest(t)
	// Should not panic or error — just a no-op
	svc.ValidateCache()
}

func TestPosterOverlayService_CacheKeyForItem(t *testing.T) {
	svc, _ := setupPosterOverlayTest(t)

	tmdbID := 12345
	item := db.SunsetQueueItem{
		MediaName: "Firefly",
		TmdbID:    &tmdbID,
	}

	key := svc.cacheKeyForItem(1, item)
	expected := poster.CacheKey(1, 12345, "orig")
	if key != expected {
		t.Errorf("Expected cache key %q, got %q", expected, key)
	}
}

func TestPosterOverlayService_CacheKeyForItem_NilTmdbID(t *testing.T) {
	svc, _ := setupPosterOverlayTest(t)

	item := db.SunsetQueueItem{
		MediaName: "Firefly",
		TmdbID:    nil,
	}

	key := svc.cacheKeyForItem(1, item)
	expected := poster.CacheKey(1, 0, "orig")
	if key != expected {
		t.Errorf("Expected cache key %q for nil TmdbID, got %q", expected, key)
	}
}

func TestPosterOverlayService_UpdateOverlay_NilTmdbID(t *testing.T) {
	svc, _ := setupPosterOverlayTest(t)

	item := db.SunsetQueueItem{
		MediaName: "Firefly",
		TmdbID:    nil,
	}

	// Should return nil without error when TmdbID is nil
	err := svc.UpdateOverlay(item, 30, PosterDeps{})
	if err != nil {
		t.Errorf("Expected nil error for nil TmdbID, got: %v", err)
	}
}

func TestPosterOverlayService_RestoreOriginal_NilTmdbID(t *testing.T) {
	svc, _ := setupPosterOverlayTest(t)

	item := db.SunsetQueueItem{
		MediaName: "Firefly",
		TmdbID:    nil,
	}

	err := svc.RestoreOriginal(item, PosterDeps{})
	if err != nil {
		t.Errorf("Expected nil error for nil TmdbID, got: %v", err)
	}
}
