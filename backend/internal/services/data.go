package services

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// PreviewCacheClearer provides the ability to clear the persisted media cache.
// Defined here to avoid import cycles between DataService and PreviewService.
type PreviewCacheClearer interface {
	ClearPersistedCache()
	InvalidatePreviewCache(reason string)
}

// DataService handles data reset operations.
type DataService struct {
	db      *gorm.DB
	bus     *events.EventBus
	preview PreviewCacheClearer
}

// NewDataService creates a new DataService.
func NewDataService(database *gorm.DB, bus *events.EventBus) *DataService {
	return &DataService{db: database, bus: bus}
}

// SetPreviewService wires the preview service dependency for cache clearing.
// Called by Registry after construction to avoid circular initialization.
func (s *DataService) SetPreviewService(preview PreviewCacheClearer) {
	s.preview = preview
}

// Reset clears all scraped data. Returns a summary of rows affected.
// This clears audit_log, approval_queue, library_histories, engine_run_stats,
// and resets transient fields on disk_groups and integration_configs.
// Lifetime stats and preferences are NOT cleared.
func (s *DataService) Reset() (map[string]int64, error) {
	summary := map[string]int64{}

	// 1. Delete all audit_log entries
	res := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&db.AuditLogEntry{})
	if res.Error != nil {
		return nil, fmt.Errorf("failed to clear audit log: %w", res.Error)
	}
	summary["auditLog"] = res.RowsAffected

	// 2. Delete all approval_queue entries
	res = s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&db.ApprovalQueueItem{})
	if res.Error != nil {
		return nil, fmt.Errorf("failed to clear approval queue: %w", res.Error)
	}
	summary["approvalQueue"] = res.RowsAffected

	// 3. Delete all library_histories
	res = s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&db.LibraryHistory{})
	if res.Error != nil {
		return nil, fmt.Errorf("failed to clear library history: %w", res.Error)
	}
	summary["libraryHistories"] = res.RowsAffected

	// 4. Delete all engine_run_stats
	res = s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&db.EngineRunStats{})
	if res.Error != nil {
		return nil, fmt.Errorf("failed to clear engine run stats: %w", res.Error)
	}
	summary["engineRunStats"] = res.RowsAffected

	// 5. Reset transient fields on disk_groups (preserve user thresholds)
	res = s.db.Model(&db.DiskGroup{}).Where("1 = 1").Updates(map[string]any{
		"total_bytes": 0,
		"used_bytes":  0,
	})
	if res.Error != nil {
		return nil, fmt.Errorf("failed to reset disk groups: %w", res.Error)
	}
	summary["diskGroupsReset"] = res.RowsAffected

	// 6. Reset transient fields on integration_configs
	res = s.db.Model(&db.IntegrationConfig{}).Where("1 = 1").Updates(map[string]any{
		"media_size_bytes": 0,
		"media_count":      0,
		"last_sync":        nil,
		"last_error":       "",
	})
	if res.Error != nil {
		return nil, fmt.Errorf("failed to reset integration stats: %w", res.Error)
	}
	summary["integrationsReset"] = res.RowsAffected

	// 7. Clear persisted media cache and in-memory preview cache
	if s.preview != nil {
		s.preview.ClearPersistedCache()
		s.preview.InvalidatePreviewCache("data_reset")
	}
	summary["mediaCacheCleared"] = int64(1)

	s.bus.Publish(events.DataResetEvent{Summary: summary})
	slog.Info("Data reset completed", "component", "services", "summary", summary)

	return summary, nil
}
