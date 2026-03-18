package routes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/testutil"
)

// ---------- tests ----------

func TestListApprovalQueue(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Seed integration + queue items
	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	if err := database.Create(&ic).Error; err != nil {
		t.Fatalf("Failed to seed integration: %v", err)
	}

	for i, name := range []string{"Movie A", "Movie B", "Movie C"} {
		item := db.ApprovalQueueItem{
			MediaName: name, MediaType: "movie", Reason: "Score: 0.80",
			SizeBytes: 1000000 * int64(i+1), IntegrationID: ic.ID,
			ExternalID: fmt.Sprintf("%d", i+1), Status: db.StatusPending,
		}
		if err := database.Create(&item).Error; err != nil {
			t.Fatalf("Failed to seed queue item: %v", err)
		}
	}

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/approval-queue", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []db.ApprovalQueueItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
}

func TestListApprovalQueue_FilterByStatus(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	// Create one pending and one rejected
	database.Create(&db.ApprovalQueueItem{
		MediaName: "Pending Movie", MediaType: "movie", Reason: "Score: 0.80",
		SizeBytes: 1000, IntegrationID: ic.ID, ExternalID: "1", Status: db.StatusPending,
	})
	snoozed := time.Now().Add(24 * time.Hour)
	database.Create(&db.ApprovalQueueItem{
		MediaName: "Rejected Movie", MediaType: "movie", Reason: "Score: 0.50",
		SizeBytes: 2000, IntegrationID: ic.ID, ExternalID: "2",
		Status: db.StatusRejected, SnoozedUntil: &snoozed,
	})

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/approval-queue?status=pending", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []db.ApprovalQueueItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("Expected 1 pending item, got %d", len(items))
	}
	if len(items) > 0 && items[0].MediaName != "Pending Movie" {
		t.Errorf("Expected 'Pending Movie', got %q", items[0].MediaName)
	}
}

func TestApproveQueueItem(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Enable deletions
	database.Model(&db.PreferenceSet{}).Where("id = 1").Update("deletions_enabled", true)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	item := db.ApprovalQueueItem{
		MediaName: "Movie to Approve", MediaType: "movie", Reason: "Score: 0.90",
		SizeBytes: 5000000, IntegrationID: ic.ID, ExternalID: "42", Status: db.StatusPending,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		fmt.Sprintf("/api/approval-queue/%d/approve", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status changed
	var approved db.ApprovalQueueItem
	database.First(&approved, item.ID)
	if approved.Status != db.StatusApproved {
		t.Errorf("Expected status %q, got %q", db.StatusApproved, approved.Status)
	}
}

func TestApproveQueueItem_DeletionsDisabled(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Ensure deletions are disabled (default)
	database.Model(&db.PreferenceSet{}).Where("id = 1").Update("deletions_enabled", false)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	item := db.ApprovalQueueItem{
		MediaName: "Movie", MediaType: "movie", Reason: "Score: 0.90",
		SizeBytes: 1000, IntegrationID: ic.ID, ExternalID: "1", Status: db.StatusPending,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		fmt.Sprintf("/api/approval-queue/%d/approve", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict when deletions disabled, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestApproveQueueItem_NotFound(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	database.Model(&db.PreferenceSet{}).Where("id = 1").Update("deletions_enabled", true)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		"/api/approval-queue/99999/approve",
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestApproveQueueItem_InvalidID(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		"/api/approval-queue/abc/approve",
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRejectQueueItem(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	item := db.ApprovalQueueItem{
		MediaName: "Movie to Reject", MediaType: "movie", Reason: "Score: 0.30",
		SizeBytes: 3000000, IntegrationID: ic.ID, ExternalID: "10", Status: db.StatusPending,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		fmt.Sprintf("/api/approval-queue/%d/reject", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status changed
	var rejected db.ApprovalQueueItem
	database.First(&rejected, item.ID)
	if rejected.Status != db.StatusRejected {
		t.Errorf("Expected status %q, got %q", db.StatusRejected, rejected.Status)
	}
	if rejected.SnoozedUntil == nil {
		t.Error("Expected SnoozedUntil to be set after rejection")
	}
}

func TestUnsnoozeQueueItem(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	snoozed := time.Now().Add(24 * time.Hour)
	item := db.ApprovalQueueItem{
		MediaName: "Snoozed Movie", MediaType: "movie", Reason: "Score: 0.40",
		SizeBytes: 2000000, IntegrationID: ic.ID, ExternalID: "20",
		Status: db.StatusRejected, SnoozedUntil: &snoozed,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		fmt.Sprintf("/api/approval-queue/%d/unsnooze", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status changed back to pending
	var unsnoozed db.ApprovalQueueItem
	database.First(&unsnoozed, item.ID)
	if unsnoozed.Status != db.StatusPending {
		t.Errorf("Expected status %q, got %q", db.StatusPending, unsnoozed.Status)
	}
}

func TestUnsnoozeQueueItem_NotRejected(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	item := db.ApprovalQueueItem{
		MediaName: "Pending Movie", MediaType: "movie", Reason: "Score: 0.80",
		SizeBytes: 1000, IntegrationID: ic.ID, ExternalID: "1", Status: db.StatusPending,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodPost,
		fmt.Sprintf("/api/approval-queue/%d/unsnooze", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for unsnooze on non-rejected item, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDismissQueueItem_Pending(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	item := db.ApprovalQueueItem{
		MediaName: "Firefly", MediaType: "show", Reason: "Score: 0.85",
		SizeBytes: 5000000, IntegrationID: ic.ID, ExternalID: "1", Status: db.StatusPending,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodDelete,
		fmt.Sprintf("/api/approval-queue/%d", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify item was deleted
	var count int64
	database.Model(&db.ApprovalQueueItem{}).Where("id = ?", item.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected item to be deleted, but it still exists")
	}
}

func TestDismissQueueItem_Rejected(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	snoozed := time.Now().Add(24 * time.Hour)
	item := db.ApprovalQueueItem{
		MediaName: "Serenity", MediaType: "movie", Reason: "Score: 0.40",
		SizeBytes: 3000000, IntegrationID: ic.ID, ExternalID: "2",
		Status: db.StatusRejected, SnoozedUntil: &snoozed,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodDelete,
		fmt.Sprintf("/api/approval-queue/%d", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify item was deleted
	var count int64
	database.Model(&db.ApprovalQueueItem{}).Where("id = ?", item.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected item to be deleted, but it still exists")
	}
}

func TestDismissQueueItem_Approved(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	item := db.ApprovalQueueItem{
		MediaName: "Firefly", MediaType: "show", Reason: "Score: 0.90",
		SizeBytes: 5000000, IntegrationID: ic.ID, ExternalID: "1", Status: db.StatusApproved,
	}
	database.Create(&item)

	req := testutil.AuthenticatedRequest(t,
		http.MethodDelete,
		fmt.Sprintf("/api/approval-queue/%d", item.ID),
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for approved item, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDismissQueueItem_NotFound(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t,
		http.MethodDelete,
		"/api/approval-queue/99999",
		nil,
	)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestClearQueue(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	ic := db.IntegrationConfig{
		Type: "sonarr", Name: "Test", URL: "http://localhost:8989", APIKey: "key",
	}
	database.Create(&ic)

	// Create 2 pending + 1 rejected items
	database.Create(&db.ApprovalQueueItem{
		MediaName: "Firefly", MediaType: "show", Reason: "Score: 0.80",
		SizeBytes: 1000, IntegrationID: ic.ID, ExternalID: "1", Status: db.StatusPending,
	})
	database.Create(&db.ApprovalQueueItem{
		MediaName: "Serenity", MediaType: "movie", Reason: "Score: 0.70",
		SizeBytes: 2000, IntegrationID: ic.ID, ExternalID: "2", Status: db.StatusPending,
	})
	snoozed := time.Now().Add(24 * time.Hour)
	database.Create(&db.ApprovalQueueItem{
		MediaName: "Firefly - Season 1", MediaType: "season", Reason: "Score: 0.50",
		SizeBytes: 3000, IntegrationID: ic.ID, ExternalID: "3",
		Status: db.StatusRejected, SnoozedUntil: &snoozed,
	})

	req := testutil.AuthenticatedRequest(t, http.MethodPost, "/api/approval-queue/clear", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	cleared, ok := result["cleared"].(float64)
	if !ok || int(cleared) != 3 {
		t.Errorf("Expected cleared=3, got %v", result["cleared"])
	}

	// Verify queue is empty
	var remaining int64
	database.Model(&db.ApprovalQueueItem{}).Count(&remaining)
	if remaining != 0 {
		t.Errorf("Expected 0 remaining items, got %d", remaining)
	}
}

func TestListApprovalQueue_RequiresAuth(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/approval-queue", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 without auth, got %d", rec.Code)
	}
}
