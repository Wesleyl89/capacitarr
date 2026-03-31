package services

import (
	"fmt"
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/integrations"
)

// mockSearcher implements integrations.NativeIDSearcher for tests.
type mockSearcher struct {
	result string
	err    error
}

func (m *mockSearcher) SearchByTMDbID(_ string, _ int) (string, error) {
	return m.result, m.err
}

var _ integrations.NativeIDSearcher = (*mockSearcher)(nil)

func TestMappingService_Resolve_Found(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Insert a mapping directly
	m := db.MediaServerMapping{
		TmdbID:        871799,
		IntegrationID: integrationID,
		NativeID:      "12345",
		MediaType:     "movie",
		Title:         "Serenity",
		UpdatedAt:     time.Now().UTC(),
	}
	if err := database.Create(&m).Error; err != nil {
		t.Fatalf("Failed to seed mapping: %v", err)
	}

	nativeID, err := svc.Resolve(871799, integrationID)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if nativeID != "12345" {
		t.Errorf("expected nativeID '12345', got %q", nativeID)
	}
}

func TestMappingService_Resolve_NotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	_, err := svc.Resolve(999999, 1)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound, got %v", err)
	}
}

func TestMappingService_ResolveAll_Mixed(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Seed two mappings
	for _, m := range []db.MediaServerMapping{
		{TmdbID: 100, IntegrationID: integrationID, NativeID: "plex-100", MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC()},
		{TmdbID: 200, IntegrationID: integrationID, NativeID: "plex-200", MediaType: "show", Title: "Firefly", UpdatedAt: time.Now().UTC()},
	} {
		if err := database.Create(&m).Error; err != nil {
			t.Fatalf("Failed to seed mapping: %v", err)
		}
	}

	// Request 3 IDs — 2 exist, 1 doesn't
	result, err := svc.ResolveAll([]int{100, 200, 300}, integrationID)
	if err != nil {
		t.Fatalf("ResolveAll returned error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[100] != "plex-100" {
		t.Errorf("expected plex-100 for TMDb 100, got %q", result[100])
	}
	if result[200] != "plex-200" {
		t.Errorf("expected plex-200 for TMDb 200, got %q", result[200])
	}
	if _, found := result[300]; found {
		t.Error("expected TMDb 300 to be absent from results")
	}
}

func TestMappingService_ResolveAll_Empty(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	result, err := svc.ResolveAll([]int{}, 1)
	if err != nil {
		t.Fatalf("ResolveAll returned error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestMappingService_BulkUpsert_Insert(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	mappings := []db.MediaServerMapping{
		{TmdbID: 100, IntegrationID: integrationID, NativeID: "plex-100", MediaType: "movie", Title: "Serenity"},
		{TmdbID: 200, IntegrationID: integrationID, NativeID: "plex-200", MediaType: "show", Title: "Firefly"},
	}

	if err := svc.BulkUpsert(mappings); err != nil {
		t.Fatalf("BulkUpsert returned error: %v", err)
	}

	// Verify both are stored
	nativeID, err := svc.Resolve(100, integrationID)
	if err != nil {
		t.Fatalf("Resolve TMDb 100 returned error: %v", err)
	}
	if nativeID != "plex-100" {
		t.Errorf("expected plex-100, got %q", nativeID)
	}

	nativeID, err = svc.Resolve(200, integrationID)
	if err != nil {
		t.Fatalf("Resolve TMDb 200 returned error: %v", err)
	}
	if nativeID != "plex-200" {
		t.Errorf("expected plex-200, got %q", nativeID)
	}
}

func TestMappingService_BulkUpsert_Update(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Insert initial mapping
	initial := []db.MediaServerMapping{
		{TmdbID: 100, IntegrationID: integrationID, NativeID: "old-id", MediaType: "movie", Title: "Serenity"},
	}
	if err := svc.BulkUpsert(initial); err != nil {
		t.Fatalf("BulkUpsert (initial) returned error: %v", err)
	}

	// Update with new native ID
	updated := []db.MediaServerMapping{
		{TmdbID: 100, IntegrationID: integrationID, NativeID: "new-id", MediaType: "movie", Title: "Serenity"},
	}
	if err := svc.BulkUpsert(updated); err != nil {
		t.Fatalf("BulkUpsert (update) returned error: %v", err)
	}

	nativeID, err := svc.Resolve(100, integrationID)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if nativeID != "new-id" {
		t.Errorf("expected new-id after update, got %q", nativeID)
	}
}

func TestMappingService_BulkUpsert_Idempotent(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	mappings := []db.MediaServerMapping{
		{TmdbID: 100, IntegrationID: integrationID, NativeID: "plex-100", MediaType: "movie", Title: "Serenity"},
	}

	// Insert twice — should not error
	if err := svc.BulkUpsert(mappings); err != nil {
		t.Fatalf("BulkUpsert (first) returned error: %v", err)
	}

	// Sleep briefly so updated_at changes
	time.Sleep(10 * time.Millisecond)

	if err := svc.BulkUpsert(mappings); err != nil {
		t.Fatalf("BulkUpsert (second) returned error: %v", err)
	}

	// Should still resolve correctly
	nativeID, err := svc.Resolve(100, integrationID)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if nativeID != "plex-100" {
		t.Errorf("expected plex-100, got %q", nativeID)
	}
}

func TestMappingService_BulkUpsert_Empty(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	// Empty batch should be a no-op
	if err := svc.BulkUpsert(nil); err != nil {
		t.Fatalf("BulkUpsert(nil) returned error: %v", err)
	}
	if err := svc.BulkUpsert([]db.MediaServerMapping{}); err != nil {
		t.Fatalf("BulkUpsert(empty) returned error: %v", err)
	}
}

func TestMappingService_DeleteStale(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Insert a mapping with old updated_at
	staleTime := time.Now().UTC().Add(-48 * time.Hour)
	m := db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "plex-100",
		MediaType: "movie", Title: "Serenity", UpdatedAt: staleTime,
	}
	if err := database.Create(&m).Error; err != nil {
		t.Fatalf("Failed to seed mapping: %v", err)
	}

	// Delete mappings older than 24 hours
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	deleted, err := svc.DeleteStale(integrationID, cutoff)
	if err != nil {
		t.Fatalf("DeleteStale returned error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify it's gone
	_, err = svc.Resolve(100, integrationID)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound after delete, got %v", err)
	}
}

func TestMappingService_GarbageCollect_MaxAge(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Insert one fresh and one stale mapping
	now := time.Now().UTC()
	fresh := db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "fresh",
		MediaType: "movie", Title: "Serenity", UpdatedAt: now,
	}
	stale := db.MediaServerMapping{
		TmdbID: 200, IntegrationID: integrationID, NativeID: "stale",
		MediaType: "show", Title: "Firefly", UpdatedAt: now.Add(-10 * 24 * time.Hour),
	}
	if err := database.Create(&fresh).Error; err != nil {
		t.Fatalf("Failed to seed fresh mapping: %v", err)
	}
	if err := database.Create(&stale).Error; err != nil {
		t.Fatalf("Failed to seed stale mapping: %v", err)
	}

	// GC with 7-day window
	deleted, err := svc.GarbageCollect(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("GarbageCollect returned error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted (stale), got %d", deleted)
	}

	// Fresh mapping should survive
	nativeID, err := svc.Resolve(100, integrationID)
	if err != nil {
		t.Fatalf("Fresh mapping should survive GC: %v", err)
	}
	if nativeID != "fresh" {
		t.Errorf("expected 'fresh', got %q", nativeID)
	}

	// Stale mapping should be gone
	_, err = svc.Resolve(200, integrationID)
	if err != ErrMappingNotFound {
		t.Errorf("stale mapping should be deleted, got %v", err)
	}
}

func TestMappingService_GarbageCollect_Orphaned(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	// Create a real integration, insert a mapping, then delete the integration.
	// This simulates an orphaned mapping left behind when an integration is removed.
	// The ON DELETE CASCADE FK should handle this automatically, but GC provides
	// belt-and-suspenders cleanup for edge cases.
	ic := db.IntegrationConfig{
		Type: "plex", Name: "Orphan Test Plex",
		URL: "http://localhost:32400", APIKey: "test-key",
	}
	if err := database.Create(&ic).Error; err != nil {
		t.Fatalf("Failed to seed integration: %v", err)
	}

	m := db.MediaServerMapping{
		TmdbID: 100, IntegrationID: ic.ID, NativeID: "orphaned",
		MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC(),
	}
	if err := database.Create(&m).Error; err != nil {
		t.Fatalf("Failed to seed mapping: %v", err)
	}

	// Verify mapping exists before deletion
	if _, err := svc.Resolve(100, ic.ID); err != nil {
		t.Fatalf("Mapping should exist before integration deletion: %v", err)
	}

	// Delete the integration — with FK enforcement, CASCADE may auto-delete.
	// If CASCADE fires, GC has nothing to do (which is the expected happy path).
	// If FK enforcement is off, GC catches the orphan.
	database.Delete(&ic)

	// GC should find and remove orphaned mappings (or confirm already cleaned by CASCADE)
	deleted, err := svc.GarbageCollect(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("GarbageCollect returned error: %v", err)
	}

	// The mapping should be gone regardless of whether CASCADE or GC removed it
	_, resolveErr := svc.Resolve(100, ic.ID)
	if resolveErr != ErrMappingNotFound {
		t.Errorf("orphaned mapping should be removed, got %v", resolveErr)
	}

	// If CASCADE already handled it, deleted will be 0. That's correct behavior —
	// GC is a safety net, not the primary cleanup mechanism.
	_ = deleted
}

func TestMappingService_Invalidate(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Insert a mapping
	m := db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "plex-100",
		MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC(),
	}
	if err := database.Create(&m).Error; err != nil {
		t.Fatalf("Failed to seed mapping: %v", err)
	}

	// Invalidate it
	if err := svc.Invalidate(100, integrationID); err != nil {
		t.Fatalf("Invalidate returned error: %v", err)
	}

	// Should be gone
	_, err := svc.Resolve(100, integrationID)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound after invalidation, got %v", err)
	}
}

func TestMappingService_GetMapping(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Insert a mapping
	m := db.MediaServerMapping{
		TmdbID: 871799, IntegrationID: integrationID, NativeID: "12345",
		MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC(),
	}
	if err := database.Create(&m).Error; err != nil {
		t.Fatalf("Failed to seed mapping: %v", err)
	}

	got, err := svc.GetMapping(871799, integrationID)
	if err != nil {
		t.Fatalf("GetMapping returned error: %v", err)
	}
	if got.NativeID != "12345" {
		t.Errorf("expected nativeID '12345', got %q", got.NativeID)
	}
	if got.Title != "Serenity" {
		t.Errorf("expected title 'Serenity', got %q", got.Title)
	}
	if got.MediaType != "movie" {
		t.Errorf("expected mediaType 'movie', got %q", got.MediaType)
	}

	// Non-existent
	_, err = svc.GetMapping(999999, integrationID)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound, got %v", err)
	}
}

func TestMappingService_TouchedBefore(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	now := time.Now().UTC()
	// Insert one fresh and one stale
	if err := database.Create(&db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "fresh",
		MediaType: "movie", Title: "Serenity", UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("Failed to seed fresh mapping: %v", err)
	}
	if err := database.Create(&db.MediaServerMapping{
		TmdbID: 200, IntegrationID: integrationID, NativeID: "stale",
		MediaType: "show", Title: "Firefly", UpdatedAt: now.Add(-48 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("Failed to seed stale mapping: %v", err)
	}

	count, err := svc.TouchedBefore(integrationID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("TouchedBefore returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 stale mapping, got %d", count)
	}
}

// ─── Phase 2: Search Fallback Tests ─────────────────────────────────────────

func TestMappingService_ResolveWithSearch_DBHit(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Seed a mapping in DB
	if err := database.Create(&db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "plex-100",
		MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("Failed to seed mapping: %v", err)
	}

	// Searcher should NOT be called when DB has the mapping
	searcher := &mockSearcher{result: "should-not-be-used"}
	nativeID, err := svc.ResolveWithSearch(100, integrationID, "Serenity", searcher)
	if err != nil {
		t.Fatalf("ResolveWithSearch returned error: %v", err)
	}
	if nativeID != "plex-100" {
		t.Errorf("expected plex-100 from DB, got %q", nativeID)
	}
}

func TestMappingService_ResolveWithSearch_SearchHit(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// No mapping in DB — searcher finds the item
	searcher := &mockSearcher{result: "plex-discovered"}
	nativeID, err := svc.ResolveWithSearch(100, integrationID, "Serenity", searcher)
	if err != nil {
		t.Fatalf("ResolveWithSearch returned error: %v", err)
	}
	if nativeID != "plex-discovered" {
		t.Errorf("expected plex-discovered from search, got %q", nativeID)
	}

	// Verify the discovered mapping was stored in DB
	stored, storeErr := svc.Resolve(100, integrationID)
	if storeErr != nil {
		t.Fatalf("Discovered mapping should be stored in DB: %v", storeErr)
	}
	if stored != "plex-discovered" {
		t.Errorf("stored mapping should be plex-discovered, got %q", stored)
	}
}

func TestMappingService_ResolveWithSearch_SearchMiss(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// No mapping in DB, searcher also fails
	searcher := &mockSearcher{err: fmt.Errorf("plex search: no item found with TMDb ID 999")}
	_, err := svc.ResolveWithSearch(999, integrationID, "Nonexistent", searcher)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound, got %v", err)
	}
}

func TestMappingService_ResolveWithSearch_NilSearcher(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// No mapping in DB, nil searcher — should return ErrMappingNotFound
	_, err := svc.ResolveWithSearch(100, integrationID, "Serenity", nil)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound with nil searcher, got %v", err)
	}
}

func TestMappingService_InvalidateAndResolve_Recovers(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Seed a stale mapping
	if err := database.Create(&db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "old-stale-id",
		MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("Failed to seed stale mapping: %v", err)
	}

	// Searcher finds the item at a new native ID
	searcher := &mockSearcher{result: "new-correct-id"}
	nativeID, err := svc.InvalidateAndResolve(100, integrationID, "Serenity", searcher)
	if err != nil {
		t.Fatalf("InvalidateAndResolve returned error: %v", err)
	}
	if nativeID != "new-correct-id" {
		t.Errorf("expected new-correct-id, got %q", nativeID)
	}

	// Verify the stale mapping was replaced
	stored, storeErr := svc.Resolve(100, integrationID)
	if storeErr != nil {
		t.Fatalf("Re-discovered mapping should be stored: %v", storeErr)
	}
	if stored != "new-correct-id" {
		t.Errorf("stored mapping should be new-correct-id, got %q", stored)
	}
}

func TestMappingService_InvalidateAndResolve_NoMatch(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	svc := NewMappingService(database, bus)

	integrationID := seedIntegration(t, database)

	// Seed a stale mapping
	if err := database.Create(&db.MediaServerMapping{
		TmdbID: 100, IntegrationID: integrationID, NativeID: "stale-id",
		MediaType: "movie", Title: "Serenity", UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("Failed to seed stale mapping: %v", err)
	}

	// Searcher also fails — item was removed from media server
	searcher := &mockSearcher{err: fmt.Errorf("plex search: no item found")}
	_, err := svc.InvalidateAndResolve(100, integrationID, "Serenity", searcher)
	if err != ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound, got %v", err)
	}

	// Stale mapping should be deleted
	_, resolveErr := svc.Resolve(100, integrationID)
	if resolveErr != ErrMappingNotFound {
		t.Errorf("stale mapping should be deleted after failed re-resolve, got %v", resolveErr)
	}
}

// ─── Phase 3: IsNotFoundError Tests ─────────────────────────────────────────

func TestIsNotFoundError_TypedError(t *testing.T) {
	err := &integrations.NotFoundError{URL: "http://plex:32400/library/metadata/123"}
	if !integrations.IsNotFoundError(err) {
		t.Error("expected IsNotFoundError to return true for *NotFoundError")
	}
}

func TestIsNotFoundError_DoAPIRequest_Format(t *testing.T) {
	// DoAPIRequest (GET) produces: "unexpected status: 404"
	err := fmt.Errorf("unexpected status: 404")
	if !integrations.IsNotFoundError(err) {
		t.Error("expected IsNotFoundError to match DoAPIRequest 404 format")
	}
}

func TestIsNotFoundError_DoAPIRequestWithBody_Format(t *testing.T) {
	// DoAPIRequestWithBody (PUT/POST) produces: "unexpected status 404: <body>"
	err := fmt.Errorf("unexpected status 404: Not Found")
	if !integrations.IsNotFoundError(err) {
		t.Error("expected IsNotFoundError to match DoAPIRequestWithBody 404 format")
	}
}

func TestIsNotFoundError_NonNotFound(t *testing.T) {
	err := fmt.Errorf("unexpected status: 500")
	if integrations.IsNotFoundError(err) {
		t.Error("expected IsNotFoundError to return false for 500")
	}
}

func TestIsNotFoundError_Nil(t *testing.T) {
	if integrations.IsNotFoundError(nil) {
		t.Error("expected IsNotFoundError to return false for nil")
	}
}
