package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
)

func TestAuditLogService_Create(t *testing.T) {
	database := setupTestDB(t)
	svc := NewAuditLogService(database)

	entry := db.AuditLogEntry{
		MediaName: "Breaking Bad",
		MediaType: "show",
		Reason:    "Score: 0.85 (WatchHistory: 1.0)",
		Action:    db.ActionDeleted,
		SizeBytes: 5069636198,
	}

	if err := svc.Create(entry); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	var count int64
	database.Model(&db.AuditLogEntry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 entry, got %d", count)
	}

	var saved db.AuditLogEntry
	database.First(&saved)
	if saved.MediaName != "Breaking Bad" {
		t.Errorf("expected media name 'Breaking Bad', got %q", saved.MediaName)
	}
	if saved.Action != db.ActionDeleted {
		t.Errorf("expected action %q, got %q", db.ActionDeleted, saved.Action)
	}
}

func TestAuditLogService_UpsertDryRun_Create(t *testing.T) {
	database := setupTestDB(t)
	svc := NewAuditLogService(database)

	entry := db.AuditLogEntry{
		MediaName: "The Wire",
		MediaType: "show",
		Reason:    "Score: 0.70",
		Action:    db.ActionDryRun,
		SizeBytes: 3000000000,
	}

	if err := svc.UpsertDryRun(entry); err != nil {
		t.Fatalf("UpsertDryRun returned error: %v", err)
	}

	var count int64
	database.Model(&db.AuditLogEntry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 entry, got %d", count)
	}
}

func TestAuditLogService_UpsertDryRun_Update(t *testing.T) {
	database := setupTestDB(t)
	svc := NewAuditLogService(database)

	// Create initial dry-run entry
	entry := db.AuditLogEntry{
		MediaName: "The Wire",
		MediaType: "show",
		Reason:    "Score: 0.70",
		Action:    db.ActionDryRun,
		SizeBytes: 3000000000,
	}
	if err := svc.UpsertDryRun(entry); err != nil {
		t.Fatalf("First UpsertDryRun failed: %v", err)
	}

	// Upsert same media with updated score
	entry.Reason = "Score: 0.85"
	entry.SizeBytes = 3500000000
	if err := svc.UpsertDryRun(entry); err != nil {
		t.Fatalf("Second UpsertDryRun failed: %v", err)
	}

	// Should still have only 1 entry
	var count int64
	database.Model(&db.AuditLogEntry{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 entry after upsert, got %d", count)
	}

	// Verify updated values
	var saved db.AuditLogEntry
	database.First(&saved)
	if saved.Reason != "Score: 0.85" {
		t.Errorf("expected updated reason, got %q", saved.Reason)
	}
	if saved.SizeBytes != 3500000000 {
		t.Errorf("expected updated size, got %d", saved.SizeBytes)
	}
}

func TestAuditLogService_PruneOlderThan(t *testing.T) {
	database := setupTestDB(t)
	svc := NewAuditLogService(database)

	// Create entries: one old, one recent
	now := time.Now().UTC()
	old := db.AuditLogEntry{
		MediaName: "Old Movie", MediaType: "movie", Reason: "Score: 0.50",
		Action: db.ActionDeleted, SizeBytes: 1000,
		CreatedAt: now.AddDate(0, 0, -60),
	}
	recent := db.AuditLogEntry{
		MediaName: "Recent Movie", MediaType: "movie", Reason: "Score: 0.90",
		Action: db.ActionDeleted, SizeBytes: 2000,
		CreatedAt: now.AddDate(0, 0, -5),
	}

	database.Create(&old)
	database.Create(&recent)

	pruned, err := svc.PruneOlderThan(30)
	if err != nil {
		t.Fatalf("PruneOlderThan returned error: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 entry pruned, got %d", pruned)
	}

	// Recent entry should remain
	var remaining []db.AuditLogEntry
	database.Find(&remaining)
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(remaining))
	}
	if remaining[0].MediaName != "Recent Movie" {
		t.Errorf("expected recent movie to remain, got %q", remaining[0].MediaName)
	}
}

func TestAuditLogService_PruneOlderThan_ZeroKeepsForever(t *testing.T) {
	database := setupTestDB(t)
	svc := NewAuditLogService(database)

	// Create an old entry
	old := db.AuditLogEntry{
		MediaName: "Ancient Movie", MediaType: "movie", Reason: "Score: 0.10",
		Action: db.ActionDeleted, SizeBytes: 1000,
		CreatedAt: time.Now().UTC().AddDate(-1, 0, 0),
	}
	database.Create(&old)

	pruned, err := svc.PruneOlderThan(0)
	if err != nil {
		t.Fatalf("PruneOlderThan(0) returned error: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 entries pruned with retention=0, got %d", pruned)
	}
}
