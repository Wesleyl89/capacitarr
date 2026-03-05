package poller

import (
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/embed" // load the embedded SQLite WASM binary
	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"capacitarr/internal/db"
)

// setupOrphanTestDB creates an in-memory SQLite database with migrations applied,
// seeds default preferences, and sets the global db.DB pointer for use by
// RecoverOrphanedApprovals (which reads from the package-level db.DB).
func setupOrphanTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(gormlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open in-memory SQLite: %v", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Seed default preferences (mirrors db.Init behaviour)
	pref := db.PreferenceSet{
		ID:                  1,
		ExecutionMode:       "approval",
		LogLevel:            "info",
		PollIntervalSeconds: 300,
	}
	if err := database.FirstOrCreate(&pref, db.PreferenceSet{ID: 1}).Error; err != nil {
		t.Fatalf("Failed to seed preferences: %v", err)
	}

	// Set the global db.DB used by RecoverOrphanedApprovals
	db.DB = database

	return database
}

// TestRecoverOrphanedApprovals verifies that RecoverOrphanedApprovals reverts
// entries with action "Approved" back to "Queued for Approval" while leaving
// entries with other statuses untouched.
func TestRecoverOrphanedApprovals(t *testing.T) {
	database := setupOrphanTestDB(t)

	integrationID := uint(1)

	// Seed entries with various statuses
	entries := []db.ApprovalQueueItem{
		{
			MediaName:     "Orphaned Movie 1",
			MediaType:     "movie",
			Reason:        "Score: 0.80",
			Status: "Approved",
			SizeBytes:     2000000000,
			IntegrationID: integrationID,
			ExternalID:    "ext-100",
			CreatedAt:     time.Now().Add(-1 * time.Hour),
		},
		{
			MediaName:     "Orphaned Movie 2",
			MediaType:     "movie",
			Reason:        "Score: 0.65",
			Status: "Approved",
			SizeBytes:     3000000000,
			IntegrationID: integrationID,
			ExternalID:    "ext-101",
			CreatedAt:     time.Now().Add(-2 * time.Hour),
		},
		{
			MediaName:     "Queued Movie",
			MediaType:     "movie",
			Reason:        "Score: 0.90",
			Status: "Queued for Approval",
			SizeBytes:     1500000000,
			IntegrationID: integrationID,
			ExternalID:    "ext-200",
			CreatedAt:     time.Now().Add(-30 * time.Minute),
		},
		{
			MediaName:     "Deleted Movie",
			MediaType:     "movie",
			Reason:        "Score: 0.95",
			Status: "Deleted",
			SizeBytes:     4000000000,
			IntegrationID: integrationID,
			ExternalID:    "ext-300",
			CreatedAt:     time.Now().Add(-3 * time.Hour),
		},
		{
			MediaName:     "Rejected Movie",
			MediaType:     "movie",
			Reason:        "Score: 0.50",
			Status: "Rejected",
			SizeBytes:     1000000000,
			IntegrationID: integrationID,
			ExternalID:    "ext-400",
			CreatedAt:     time.Now().Add(-4 * time.Hour),
		},
	}

	for i, entry := range entries {
		if err := database.Create(&entry).Error; err != nil {
			t.Fatalf("Failed to seed audit entry %d: %v", i, err)
		}
	}

	// Verify initial state: 2 "Approved" entries exist
	var approvedCount int64
	database.Model(&db.ApprovalQueueItem{}).Where("action = ?", "Approved").Count(&approvedCount)
	if approvedCount != 2 {
		t.Fatalf("Expected 2 Approved entries before recovery, got %d", approvedCount)
	}

	// Run the recovery function
	RecoverOrphanedApprovals()

	// Verify: "Approved" entries were reverted to "Queued for Approval"
	database.Model(&db.ApprovalQueueItem{}).Where("action = ?", "Approved").Count(&approvedCount)
	if approvedCount != 0 {
		t.Errorf("Expected 0 Approved entries after recovery, got %d", approvedCount)
	}

	// Verify: the two formerly-Approved entries are now "Queued for Approval"
	var queuedCount int64
	database.Model(&db.ApprovalQueueItem{}).Where("action = ?", "Queued for Approval").Count(&queuedCount)
	if queuedCount != 3 { // 2 recovered + 1 originally queued
		t.Errorf("Expected 3 'Queued for Approval' entries (2 recovered + 1 original), got %d", queuedCount)
	}

	// Verify: "Deleted" entry was NOT modified
	var deleted db.ApprovalQueueItem
	database.Where("media_name = ?", "Deleted Movie").First(&deleted)
	if deleted.Status != "Deleted" {
		t.Errorf("Expected 'Deleted' entry to be unchanged, got action %q", deleted.Status)
	}

	// Verify: "Rejected" entry was NOT modified
	var rejected db.ApprovalQueueItem
	database.Where("media_name = ?", "Rejected Movie").First(&rejected)
	if rejected.Status != "Rejected" {
		t.Errorf("Expected 'Rejected' entry to be unchanged, got action %q", rejected.Status)
	}

	// Verify: originally "Queued for Approval" entry was NOT modified
	var queued db.ApprovalQueueItem
	database.Where("media_name = ?", "Queued Movie").First(&queued)
	if queued.Status != "Queued for Approval" {
		t.Errorf("Expected 'Queued for Approval' entry to be unchanged, got action %q", queued.Status)
	}
}
