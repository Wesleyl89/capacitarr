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

// setupEvaluateTestDB creates an in-memory SQLite database with migrations applied,
// seeds default preferences, and sets the global db.DB pointer.
func setupEvaluateTestDB(t *testing.T) *gorm.DB {
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

	pref := db.PreferenceSet{
		ID:                  1,
		ExecutionMode:       "approval",
		LogLevel:            "info",
		PollIntervalSeconds: 300,
	}
	if err := database.FirstOrCreate(&pref, db.PreferenceSet{ID: 1}).Error; err != nil {
		t.Fatalf("Failed to seed preferences: %v", err)
	}

	db.DB = database
	return database
}

// TestApprovalDedup_SingleEntry verifies that running the approval dedup logic
// twice for the same media item produces only one "Queued for Approval" audit
// entry, with the second run updating the existing entry rather than creating
// a duplicate.
func TestApprovalDedup_SingleEntry(t *testing.T) {
	database := setupEvaluateTestDB(t)

	mediaName := "Adventure Time - Season 1"
	mediaType := "season"
	actionName := "Queued for Approval"
	integrationID := uint(1)

	// Simulate first engine run: create initial entry
	firstEntry := db.AuditLogEntry{
		MediaName:     mediaName,
		MediaType:     mediaType,
		Reason:        "Score: 5.50 (high score)",
		ScoreDetails:  `[{"name":"size","contribution":3.0},{"name":"age","contribution":2.5}]`,
		Action:        actionName,
		SizeBytes:     1000000000,
		IntegrationID: &integrationID,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
	}

	// Run the dedup logic (mirrors evaluate.go approval dedup path)
	var existing db.AuditLogEntry
	result := db.DB.Where(
		"media_name = ? AND media_type = ? AND action = ?",
		mediaName, mediaType, actionName,
	).First(&existing)
	if result.Error == nil {
		db.DB.Model(&existing).Updates(map[string]interface{}{
			"reason":         firstEntry.Reason,
			"score_details":  firstEntry.ScoreDetails,
			"size_bytes":     firstEntry.SizeBytes,
			"created_at":     firstEntry.CreatedAt,
			"external_id":    firstEntry.IntegrationID,
			"integration_id": firstEntry.IntegrationID,
		})
	} else {
		db.DB.Create(&firstEntry)
	}

	// Verify: one entry exists
	var count int64
	database.Model(&db.AuditLogEntry{}).Where("media_name = ? AND action = ?", mediaName, actionName).Count(&count)
	if count != 1 {
		t.Fatalf("Expected 1 audit entry after first run, got %d", count)
	}

	// Simulate second engine run: updated score and size
	secondEntry := db.AuditLogEntry{
		MediaName:     mediaName,
		MediaType:     mediaType,
		Reason:        "Score: 6.20 (higher score)",
		ScoreDetails:  `[{"name":"size","contribution":3.5},{"name":"age","contribution":2.7}]`,
		Action:        actionName,
		SizeBytes:     1100000000,
		IntegrationID: &integrationID,
		CreatedAt:     time.Now(),
	}

	// Run the dedup logic again (should update, not create)
	var existing2 db.AuditLogEntry
	result2 := db.DB.Where(
		"media_name = ? AND media_type = ? AND action = ?",
		mediaName, mediaType, actionName,
	).First(&existing2)
	if result2.Error == nil {
		db.DB.Model(&existing2).Updates(map[string]interface{}{
			"reason":         secondEntry.Reason,
			"score_details":  secondEntry.ScoreDetails,
			"size_bytes":     secondEntry.SizeBytes,
			"created_at":     secondEntry.CreatedAt,
			"external_id":    secondEntry.IntegrationID,
			"integration_id": secondEntry.IntegrationID,
		})
	} else {
		db.DB.Create(&secondEntry)
	}

	// Verify: still only one entry
	database.Model(&db.AuditLogEntry{}).Where("media_name = ? AND action = ?", mediaName, actionName).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 audit entry after second run (dedup), got %d", count)
	}

	// Verify: the entry was updated with the new values
	var updated db.AuditLogEntry
	database.Where("media_name = ? AND action = ?", mediaName, actionName).First(&updated)
	if updated.Reason != "Score: 6.20 (higher score)" {
		t.Errorf("Expected updated reason, got %q", updated.Reason)
	}
	if updated.SizeBytes != 1100000000 {
		t.Errorf("Expected updated sizeBytes=1100000000, got %d", updated.SizeBytes)
	}
}

// TestApprovalDedup_DoesNotTouchApproved verifies that the dedup logic does
// NOT overwrite entries whose action has been changed to "Approved" by the user.
func TestApprovalDedup_DoesNotTouchApproved(t *testing.T) {
	database := setupEvaluateTestDB(t)

	mediaName := "Breaking Bad - Season 1"
	mediaType := "season"
	integrationID := uint(1)

	// Create an entry that was approved by the user
	approvedEntry := db.AuditLogEntry{
		MediaName:     mediaName,
		MediaType:     mediaType,
		Reason:        "Score: 4.00 (approved)",
		ScoreDetails:  `[]`,
		Action:        "Approved",
		SizeBytes:     500000000,
		IntegrationID: &integrationID,
		CreatedAt:     time.Now().Add(-30 * time.Minute),
	}
	database.Create(&approvedEntry)

	// Now simulate the engine trying to re-queue this item for approval
	newEntry := db.AuditLogEntry{
		MediaName:     mediaName,
		MediaType:     mediaType,
		Reason:        "Score: 4.50 (re-evaluated)",
		ScoreDetails:  `[{"name":"size","contribution":4.5}]`,
		Action:        "Queued for Approval",
		SizeBytes:     550000000,
		IntegrationID: &integrationID,
		CreatedAt:     time.Now(),
	}

	// Run the approval dedup logic (WHERE action = "Queued for Approval")
	var existing db.AuditLogEntry
	result := db.DB.Where(
		"media_name = ? AND media_type = ? AND action = ?",
		mediaName, mediaType, "Queued for Approval",
	).First(&existing)
	if result.Error == nil {
		db.DB.Model(&existing).Updates(map[string]interface{}{
			"reason":         newEntry.Reason,
			"score_details":  newEntry.ScoreDetails,
			"size_bytes":     newEntry.SizeBytes,
			"created_at":     newEntry.CreatedAt,
			"external_id":    newEntry.IntegrationID,
			"integration_id": newEntry.IntegrationID,
		})
	} else {
		// No existing "Queued for Approval" entry found — create a new one
		db.DB.Create(&newEntry)
	}

	// Verify: the approved entry is untouched
	var approved db.AuditLogEntry
	database.Where("media_name = ? AND action = ?", mediaName, "Approved").First(&approved)
	if approved.ID == 0 {
		t.Fatal("Expected approved entry to still exist")
	}
	if approved.Reason != "Score: 4.00 (approved)" {
		t.Errorf("Expected approved entry reason untouched, got %q", approved.Reason)
	}
	if approved.SizeBytes != 500000000 {
		t.Errorf("Expected approved entry sizeBytes untouched, got %d", approved.SizeBytes)
	}

	// Verify: a new "Queued for Approval" entry was created (separate from the approved one)
	var queued db.AuditLogEntry
	database.Where("media_name = ? AND action = ?", mediaName, "Queued for Approval").First(&queued)
	if queued.ID == 0 {
		t.Fatal("Expected new 'Queued for Approval' entry to be created")
	}
	if queued.Reason != "Score: 4.50 (re-evaluated)" {
		t.Errorf("Expected new queued entry reason, got %q", queued.Reason)
	}

	// Verify: total entries = 2 (one approved, one queued)
	var total int64
	database.Model(&db.AuditLogEntry{}).Where("media_name = ?", mediaName).Count(&total)
	if total != 2 {
		t.Errorf("Expected 2 total entries (1 approved + 1 queued), got %d", total)
	}
}
