package services

import (
	"fmt"
	"log/slog"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"

	"gorm.io/gorm"
)

// SunsetService manages the sunset queue lifecycle. Handles countdown tracking,
// cancellation, rescheduling, expiry processing, escalation, and label management.
// Does NOT perform deletions — hands items to DeletionService when countdowns expire.
//
// Follows the established service pattern: accepts *gorm.DB and *events.EventBus
// in constructor; registered on services.Registry as reg.Sunset.
type SunsetService struct {
	db  *gorm.DB
	bus *events.EventBus
}

// SunsetDeps holds service dependencies for label management and deletion handoff.
// Follows the same pattern as ExecuteApprovalDeps in approval.go.
// SettingsReader is the existing interface defined in deletion.go — reuse it.
type SunsetDeps struct {
	Registry       *integrations.IntegrationRegistry
	Deletion       *DeletionService
	Engine         *EngineService
	Settings       SettingsReader
	PosterOverlay  *PosterOverlayService   // Optional: if set, posters are restored on cancel/expire/escalate
	TMDbToNativeID map[uint]map[int]string // integrationID → (TMDb ID → native ID); built once per cycle via IntegrationRegistry.BuildTMDbToNativeIDMaps()
}

// NewSunsetService creates a new sunset queue service.
func NewSunsetService(database *gorm.DB, bus *events.EventBus) *SunsetService {
	return &SunsetService{db: database, bus: bus}
}

// QueueSunset creates a new sunset_queue entry with a deletion_date.
// Also applies the sunset label to the item in all enabled media servers.
func (s *SunsetService) QueueSunset(item db.SunsetQueueItem, deps SunsetDeps) error {
	if err := s.db.Create(&item).Error; err != nil {
		return fmt.Errorf("create sunset queue item: %w", err)
	}

	// Apply label to media servers
	if deps.Registry != nil && deps.Settings != nil {
		if prefs, err := deps.Settings.GetPreferences(); err == nil && prefs.SunsetLabel != "" {
			if s.applyLabel(item, prefs.SunsetLabel, deps.Registry, deps.TMDbToNativeID) {
				item.LabelApplied = true
				s.db.Model(&item).Update("label_applied", true)
			}
		}
	}

	daysRemaining := s.DaysRemaining(item)
	s.bus.Publish(events.SunsetCreatedEvent{
		MediaName:     item.MediaName,
		MediaType:     item.MediaType,
		DiskGroupID:   item.DiskGroupID,
		DaysRemaining: daysRemaining,
		DeletionDate:  item.DeletionDate.Format("2006-01-02"),
	})

	return nil
}

// BulkQueueSunset creates multiple sunset entries in a transaction.
// Applies labels to all items in enabled media servers.
func (s *SunsetService) BulkQueueSunset(items []db.SunsetQueueItem, deps SunsetDeps) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	var prefs db.PreferenceSet
	if deps.Settings != nil {
		var err error
		prefs, err = deps.Settings.GetPreferences()
		if err != nil {
			return 0, fmt.Errorf("load preferences: %w", err)
		}
	}

	created := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for i := range items {
			if err := tx.Create(&items[i]).Error; err != nil {
				return fmt.Errorf("create item %d: %w", i, err)
			}
			created++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Apply labels outside the transaction (media server calls shouldn't block DB)
	if deps.Registry != nil && prefs.SunsetLabel != "" {
		for i := range items {
			if s.applyLabel(items[i], prefs.SunsetLabel, deps.Registry, deps.TMDbToNativeID) {
				items[i].LabelApplied = true
				s.db.Model(&items[i]).Update("label_applied", true)
			}
		}
	}

	// Publish events
	for _, item := range items {
		s.bus.Publish(events.SunsetCreatedEvent{
			MediaName:     item.MediaName,
			MediaType:     item.MediaType,
			DiskGroupID:   item.DiskGroupID,
			DaysRemaining: s.DaysRemaining(item),
			DeletionDate:  item.DeletionDate.Format("2006-01-02"),
		})
	}

	return created, nil
}

// Cancel removes a sunset item. Removes the label from media servers.
// Publishes sunset_cancelled event.
func (s *SunsetService) Cancel(entryID uint, deps SunsetDeps) error {
	var item db.SunsetQueueItem
	if err := s.db.First(&item, entryID).Error; err != nil {
		return fmt.Errorf("find sunset item %d: %w", entryID, err)
	}

	// Restore poster overlay
	if item.PosterOverlayActive && deps.PosterOverlay != nil && deps.Registry != nil {
		if err := deps.PosterOverlay.RestoreOriginal(item, PosterDeps{Registry: deps.Registry, TMDbToNativeID: deps.TMDbToNativeID}); err != nil {
			slog.Warn("Failed to restore poster on cancel",
				"component", "services", "mediaName", item.MediaName, "error", err)
		}
	}

	// Remove label from media servers
	if item.LabelApplied && deps.Registry != nil && deps.Settings != nil {
		if prefs, err := deps.Settings.GetPreferences(); err == nil && prefs.SunsetLabel != "" {
			s.removeLabel(item, prefs.SunsetLabel, deps.Registry, deps.TMDbToNativeID)
		}
	}

	if err := s.db.Delete(&item).Error; err != nil {
		return fmt.Errorf("delete sunset item %d: %w", entryID, err)
	}

	s.bus.Publish(events.SunsetCancelledEvent{
		MediaName:   item.MediaName,
		MediaType:   item.MediaType,
		DiskGroupID: item.DiskGroupID,
	})

	return nil
}

// Reschedule updates the deletion_date for a sunset item.
func (s *SunsetService) Reschedule(entryID uint, newDate time.Time) (*db.SunsetQueueItem, error) {
	var item db.SunsetQueueItem
	if err := s.db.First(&item, entryID).Error; err != nil {
		return nil, fmt.Errorf("find sunset item %d: %w", entryID, err)
	}

	item.DeletionDate = newDate
	if err := s.db.Save(&item).Error; err != nil {
		return nil, fmt.Errorf("update sunset item %d: %w", entryID, err)
	}

	s.bus.Publish(events.SunsetRescheduledEvent{
		MediaName:        item.MediaName,
		MediaType:        item.MediaType,
		DiskGroupID:      item.DiskGroupID,
		NewDaysRemaining: s.DaysRemaining(item),
		NewDeletionDate:  newDate.Format("2006-01-02"),
	})

	return &item, nil
}

// ListForDiskGroup returns all sunset items for a given disk group.
func (s *SunsetService) ListForDiskGroup(diskGroupID uint) ([]db.SunsetQueueItem, error) {
	var items []db.SunsetQueueItem
	err := s.db.Where("disk_group_id = ?", diskGroupID).Order("deletion_date ASC").Find(&items).Error
	return items, err
}

// ListAll returns all sunset items across all disk groups, ordered by deletion_date.
func (s *SunsetService) ListAll() ([]db.SunsetQueueItem, error) {
	var items []db.SunsetQueueItem
	err := s.db.Order("deletion_date ASC").Find(&items).Error
	return items, err
}

// GetExpired returns items where deletion_date <= now that have not already
// been handed to DeletionService (expired_at IS NULL).
func (s *SunsetService) GetExpired() ([]db.SunsetQueueItem, error) {
	var items []db.SunsetQueueItem
	err := s.db.Where("deletion_date <= ? AND expired_at IS NULL", time.Now().UTC()).Order("deletion_date ASC").Find(&items).Error
	return items, err
}

// DaysRemaining calculates the countdown for a given sunset item.
// Returns 0 if the deletion date is in the past.
func (s *SunsetService) DaysRemaining(item db.SunsetQueueItem) int {
	remaining := int(time.Until(item.DeletionDate).Hours() / 24)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ProcessExpired transitions all expired items to DeletionService.
// Removes labels from media servers. Called by the daily cron job.
func (s *SunsetService) ProcessExpired(deps SunsetDeps) (int, error) {
	expired, err := s.GetExpired()
	if err != nil {
		return 0, fmt.Errorf("get expired items: %w", err)
	}
	if len(expired) == 0 {
		return 0, nil
	}

	var prefs db.PreferenceSet
	if deps.Settings != nil {
		var prefsErr error
		prefs, prefsErr = deps.Settings.GetPreferences()
		if prefsErr != nil {
			slog.Warn("Failed to load preferences for sunset expiry — label removal may be skipped",
				"component", "services", "error", prefsErr)
		}
	}

	processed := 0
	for _, item := range expired {
		if s.processExpiredItem(item, prefs, deps) {
			processed++
		}
	}

	return processed, nil
}

// Escalate force-expires sunset items for a disk group during threshold breach.
// Processes expired first, then oldest-in-queue, freeing only enough to reach
// targetBytes. Returns bytes freed.
func (s *SunsetService) Escalate(diskGroupID uint, targetBytes int64, deps SunsetDeps) (int64, error) {
	var freedBytes int64
	itemsExpired := 0

	var prefs db.PreferenceSet
	if deps.Settings != nil {
		var prefsErr error
		prefs, prefsErr = deps.Settings.GetPreferences()
		if prefsErr != nil {
			slog.Warn("Failed to load preferences for sunset escalation — label removal may be skipped",
				"component", "services", "error", prefsErr)
		}
	}

	// Step 1: Delete already-expired items first (skip those already handed off)
	var expired []db.SunsetQueueItem
	s.db.Where("disk_group_id = ? AND deletion_date <= ? AND expired_at IS NULL", diskGroupID, time.Now().UTC()).
		Order("deletion_date ASC").Find(&expired)

	for _, item := range expired {
		if freedBytes >= targetBytes {
			break
		}
		if s.processExpiredItem(item, prefs, deps) {
			freedBytes += item.SizeBytes
			itemsExpired++
		}
	}

	if freedBytes >= targetBytes {
		s.publishEscalationEvent(diskGroupID, itemsExpired, freedBytes)
		return freedBytes, nil
	}

	// Step 2: Delete oldest items (longest in queue, most warning given)
	var oldest []db.SunsetQueueItem
	s.db.Where("disk_group_id = ? AND deletion_date > ? AND expired_at IS NULL", diskGroupID, time.Now().UTC()).
		Order("created_at ASC").Find(&oldest)

	for _, item := range oldest {
		if freedBytes >= targetBytes {
			break
		}
		if s.processExpiredItem(item, prefs, deps) {
			freedBytes += item.SizeBytes
			itemsExpired++
		}
	}

	s.publishEscalationEvent(diskGroupID, itemsExpired, freedBytes)
	return freedBytes, nil
}

// CancelAll cancels all sunset items (emergency button). Returns count removed.
func (s *SunsetService) CancelAll(deps SunsetDeps) (int, error) {
	items, err := s.ListAll()
	if err != nil {
		return 0, err
	}

	for _, item := range items {
		// Restore poster overlays before clearing
		if item.PosterOverlayActive && deps.PosterOverlay != nil && deps.Registry != nil {
			if err := deps.PosterOverlay.RestoreOriginal(item, PosterDeps{Registry: deps.Registry, TMDbToNativeID: deps.TMDbToNativeID}); err != nil {
				slog.Warn("Failed to restore poster during clear-all",
					"component", "services", "mediaName", item.MediaName, "error", err)
			}
		}
	}

	if deps.Registry != nil && deps.Settings != nil {
		if prefs, prefsErr := deps.Settings.GetPreferences(); prefsErr == nil && prefs.SunsetLabel != "" {
			for _, item := range items {
				if item.LabelApplied {
					s.removeLabel(item, prefs.SunsetLabel, deps.Registry, deps.TMDbToNativeID)
				}
			}
		}
	}

	result := s.db.Where("1 = 1").Delete(&db.SunsetQueueItem{})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// CancelAllForDiskGroup cancels all sunset items for a specific disk group.
func (s *SunsetService) CancelAllForDiskGroup(diskGroupID uint, deps SunsetDeps) (int, error) {
	items, err := s.ListForDiskGroup(diskGroupID)
	if err != nil {
		return 0, err
	}

	for _, item := range items {
		// Restore poster overlays before clearing
		if item.PosterOverlayActive && deps.PosterOverlay != nil && deps.Registry != nil {
			if err := deps.PosterOverlay.RestoreOriginal(item, PosterDeps{Registry: deps.Registry, TMDbToNativeID: deps.TMDbToNativeID}); err != nil {
				slog.Warn("Failed to restore poster during disk-group clear",
					"component", "services", "mediaName", item.MediaName, "error", err)
			}
		}
	}

	if deps.Registry != nil && deps.Settings != nil {
		if prefs, prefsErr := deps.Settings.GetPreferences(); prefsErr == nil && prefs.SunsetLabel != "" {
			for _, item := range items {
				if item.LabelApplied {
					s.removeLabel(item, prefs.SunsetLabel, deps.Registry, deps.TMDbToNativeID)
				}
			}
		}
	}

	result := s.db.Where("disk_group_id = ?", diskGroupID).Delete(&db.SunsetQueueItem{})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// RemoveCompleted hard-deletes a sunset queue item after the file has been
// successfully deleted by DeletionService. No label removal or poster restore
// is needed — processExpiredItem already handled those before handoff.
func (s *SunsetService) RemoveCompleted(id uint) error {
	return s.db.Delete(&db.SunsetQueueItem{}, id).Error
}

// IsSunsetted checks if a media item is already in the sunset queue.
func (s *SunsetService) IsSunsetted(mediaName, mediaType string, diskGroupID uint) bool {
	var count int64
	s.db.Model(&db.SunsetQueueItem{}).
		Where("media_name = ? AND media_type = ? AND disk_group_id = ?", mediaName, mediaType, diskGroupID).
		Count(&count)
	return count > 0
}

// ListSunsettedKeys returns "mediaName|mediaType" keys for O(1) lookups.
// Same pattern as ApprovalService.ListSnoozedKeys().
func (s *SunsetService) ListSunsettedKeys(diskGroupID uint) (map[string]bool, error) {
	var items []db.SunsetQueueItem
	err := s.db.Select("media_name, media_type").
		Where("disk_group_id = ?", diskGroupID).Find(&items).Error
	if err != nil {
		return nil, err
	}

	keys := make(map[string]bool, len(items))
	for _, item := range items {
		keys[item.MediaName+"|"+item.MediaType] = true
	}
	return keys, nil
}

// MigrateLabel removes the old label and applies the new label across all
// queued items in all enabled media servers. Called from the settings save
// handler when the user changes SunsetLabel.
func (s *SunsetService) MigrateLabel(oldLabel, newLabel string, registry *integrations.IntegrationRegistry) error {
	var items []db.SunsetQueueItem
	if err := s.db.Where("label_applied = ?", true).Find(&items).Error; err != nil {
		return fmt.Errorf("list labeled items: %w", err)
	}

	var tmdbMaps map[uint]map[int]string
	if registry != nil {
		tmdbMaps = registry.BuildTMDbToNativeIDMaps()
	}

	for _, item := range items {
		s.removeLabel(item, oldLabel, registry, tmdbMaps)
		s.applyLabel(item, newLabel, registry, tmdbMaps)
	}

	slog.Info("Migrated sunset labels", "component", "services",
		"oldLabel", oldLabel, "newLabel", newLabel, "items", len(items))
	return nil
}

// ─── Internal helpers ───────────────────────────────────────────────────────

// applyLabel applies the sunset label to an item across all LabelManager-capable
// media servers. Returns true if at least one media server succeeded.
// Errors are logged but not propagated (label is best-effort).
func (s *SunsetService) applyLabel(item db.SunsetQueueItem, label string, registry *integrations.IntegrationRegistry, tmdbMaps map[uint]map[int]string) bool {
	if registry == nil || item.TmdbID == nil {
		return false
	}
	anySuccess := false
	for integrationID, mgr := range registry.LabelManagers() {
		idMap := tmdbMaps[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}
		if err := mgr.AddLabel(nativeID, label); err != nil {
			slog.Warn("Failed to apply sunset label",
				"component", "services", "mediaName", item.MediaName,
				"integrationID", integrationID, "error", err)
			s.bus.Publish(events.SunsetLabelFailedEvent{
				MediaName: item.MediaName, IntegrationID: integrationID,
				Label: label, Error: err.Error(),
			})
			continue
		}
		anySuccess = true
		s.bus.Publish(events.SunsetLabelAppliedEvent{
			MediaName: item.MediaName, IntegrationID: integrationID, Label: label,
		})
	}
	return anySuccess
}

// removeLabel removes the sunset label from an item across all LabelManager-capable
// media servers.
func (s *SunsetService) removeLabel(item db.SunsetQueueItem, label string, registry *integrations.IntegrationRegistry, tmdbMaps map[uint]map[int]string) {
	if registry == nil || item.TmdbID == nil {
		return
	}
	for integrationID, mgr := range registry.LabelManagers() {
		idMap := tmdbMaps[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}
		if err := mgr.RemoveLabel(nativeID, label); err != nil {
			slog.Warn("Failed to remove sunset label",
				"component", "services", "mediaName", item.MediaName,
				"integrationID", integrationID, "error", err)
			s.bus.Publish(events.SunsetLabelFailedEvent{
				MediaName: item.MediaName, IntegrationID: integrationID,
				Label: label, Error: err.Error(),
			})
			continue
		}
		s.bus.Publish(events.SunsetLabelRemovedEvent{
			MediaName: item.MediaName, IntegrationID: integrationID, Label: label,
		})
	}
}

// processExpiredItem handles a single expired/escalated item: restores poster,
// removes label, queues for deletion, and marks as expired. The item is NOT
// deleted from sunset_queue — it remains visible in the dashboard until the
// user removes it via Cancel or Clear All. The ExpiredAt timestamp prevents
// re-processing on subsequent engine cycles and cron runs.
//
// If deletion handoff fails (no registry or deleter unavailable), the item is
// NOT marked expired — it will be retried on the next cron run.
func (s *SunsetService) processExpiredItem(item db.SunsetQueueItem, prefs db.PreferenceSet, deps SunsetDeps) bool {
	// Skip if already expired (handed to DeletionService in a prior cycle)
	if item.ExpiredAt != nil {
		return false
	}

	// Restore poster overlay before deletion
	if item.PosterOverlayActive && deps.PosterOverlay != nil && deps.Registry != nil {
		if err := deps.PosterOverlay.RestoreOriginal(item, PosterDeps{Registry: deps.Registry, TMDbToNativeID: deps.TMDbToNativeID}); err != nil {
			slog.Warn("Failed to restore poster before expiry/escalation",
				"component", "services", "mediaName", item.MediaName, "error", err)
		}
	}

	// Remove label
	if item.LabelApplied && deps.Registry != nil && prefs.SunsetLabel != "" {
		s.removeLabel(item, prefs.SunsetLabel, deps.Registry, deps.TMDbToNativeID)
	}

	// Hand off to DeletionService — if this fails, keep item in queue for retry
	if deps.Deletion == nil || deps.Registry == nil {
		slog.Warn("Skipping sunset item expiry — deletion service or registry unavailable (will retry)",
			"component", "services", "mediaName", item.MediaName)
		return false
	}

	deleter, err := deps.Registry.Deleter(item.IntegrationID)
	if err != nil {
		slog.Error("Failed to get deleter for sunset item (will retry)",
			"component", "services", "mediaName", item.MediaName, "error", err)
		return false
	}

	_ = deps.Deletion.QueueDeletion(DeleteJob{
		Client: deleter,
		Item: integrations.MediaItem{
			Title:      item.MediaName,
			Type:       integrations.MediaType(item.MediaType),
			SizeBytes:  item.SizeBytes,
			ExternalID: item.ExternalID,
		},
		DiskGroupID:       &item.DiskGroupID,
		Trigger:           db.TriggerEngine,
		CollectionGroup:   item.CollectionGroup,
		EnqueuedMode:      db.ModeSunset,
		SunsetQueueItemID: item.ID,
		Score:             item.Score,
	})

	s.bus.Publish(events.SunsetExpiredEvent{
		MediaName:   item.MediaName,
		MediaType:   item.MediaType,
		DiskGroupID: item.DiskGroupID,
		SizeBytes:   item.SizeBytes,
	})

	// Mark as expired — item stays in sunset_queue for dashboard visibility
	now := time.Now().UTC()
	s.db.Model(&item).Update("expired_at", now)
	return true
}

// ValidateSunsetConfig validates sunset-mode configuration on a disk group.
// Returns an error if the configuration is invalid:
//   - mode is "sunset" but sunsetPct is nil → must be explicitly configured
//   - mode is "sunset" and sunsetPct >= targetPct → ordering violated
//   - mode is "sunset" and sunsetPct >= thresholdPct → ordering violated
//
// This is called from the disk group update path to reject invalid configs
// at save-time rather than silently failing at engine evaluation time.
func ValidateSunsetConfig(mode string, sunsetPct *float64, targetPct, thresholdPct float64) error {
	if mode != db.ModeSunset {
		return nil
	}
	if sunsetPct == nil {
		return fmt.Errorf("sunset mode requires a sunset threshold to be configured")
	}
	if *sunsetPct >= targetPct {
		return fmt.Errorf("sunset threshold (%.1f%%) must be less than target threshold (%.1f%%)", *sunsetPct, targetPct)
	}
	if *sunsetPct >= thresholdPct {
		return fmt.Errorf("sunset threshold (%.1f%%) must be less than critical threshold (%.1f%%)", *sunsetPct, thresholdPct)
	}
	return nil
}

// publishEscalationEvent publishes the escalation summary.
func (s *SunsetService) publishEscalationEvent(diskGroupID uint, itemsExpired int, bytesFreed int64) {
	if itemsExpired > 0 {
		s.bus.Publish(events.SunsetEscalatedEvent{
			DiskGroupID:  diskGroupID,
			ItemsExpired: itemsExpired,
			BytesFreed:   bytesFreed,
		})
	}
}
