package services

import (
	"fmt"
	"log/slog"
	"strings"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
	"capacitarr/internal/poster"

	"gorm.io/gorm"
)

// PosterOverlayService manages poster countdown overlays for sunset queue items.
// Downloads original posters, composites "Leaving in X days" banners, uploads
// modified posters, and restores originals on cancel/expire/escalation.
//
// Follows the established service pattern: accepts *gorm.DB and *events.EventBus.
type PosterOverlayService struct {
	db    *gorm.DB
	bus   *events.EventBus
	cache *poster.Cache
}

// PosterDeps holds dependencies for poster overlay operations.
type PosterDeps struct {
	Registry       *integrations.IntegrationRegistry
	TMDbToNativeID map[uint]map[int]string // integrationID → (TMDb ID → native ID); built via IntegrationRegistry.BuildTMDbToNativeIDMaps()
}

// NewPosterOverlayService creates a new poster overlay service with a filesystem
// cache at the given directory (typically /config/posters/originals/).
func NewPosterOverlayService(database *gorm.DB, bus *events.EventBus, cacheDir string) (*PosterOverlayService, error) {
	cache, err := poster.NewCache(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("init poster cache: %w", err)
	}
	return &PosterOverlayService{db: database, bus: bus, cache: cache}, nil
}

// UpdateOverlay downloads the original poster (if not cached), composites the
// countdown overlay, and uploads it to all enabled media servers.
func (s *PosterOverlayService) UpdateOverlay(item db.SunsetQueueItem, daysRemaining int, deps PosterDeps) error {
	if deps.Registry == nil || item.TmdbID == nil {
		return nil
	}

	managers := deps.Registry.PosterManagers()
	if len(managers) == 0 {
		return nil
	}

	for integrationID, mgr := range managers {
		// Resolve TMDb ID → native ID for this specific integration
		idMap := deps.TMDbToNativeID[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}

		cacheKey := s.cacheKeyForItem(integrationID, item)

		// Download the current poster from the media server
		currentData, _, dlErr := mgr.GetPosterImage(nativeID)
		if dlErr != nil {
			slog.Warn("Failed to download poster for overlay",
				"component", "services", "mediaName", item.MediaName,
				"integrationID", integrationID, "error", dlErr)
			s.bus.Publish(events.PosterOverlayFailedEvent{
				MediaName: item.MediaName, IntegrationID: integrationID,
				Error: fmt.Sprintf("download: %v", dlErr),
			})
			continue
		}

		// Content hash comparison: if a cached original exists, check whether
		// the user changed the poster since we cached it. If the hash differs,
		// re-cache so RestoreOriginal returns the user's new poster, not a stale one.
		if s.cache.Has(cacheKey) {
			cachedData, found, _ := s.cache.Get(cacheKey)
			if found && poster.ContentHash(cachedData) != poster.ContentHash(currentData) {
				slog.Info("Poster changed since last cache — re-caching original",
					"component", "services", "mediaName", item.MediaName,
					"integrationID", integrationID)
				if err := s.cache.Store(cacheKey, currentData); err != nil {
					slog.Warn("Failed to re-cache poster",
						"component", "services", "mediaName", item.MediaName, "error", err)
				}
			}
		} else {
			// First time — cache the original
			if err := s.cache.Store(cacheKey, currentData); err != nil {
				slog.Warn("Failed to cache original poster",
					"component", "services", "mediaName", item.MediaName, "error", err)
				continue
			}
		}

		// Read cached original for composition
		originalData, found, err := s.cache.Get(cacheKey)
		if err != nil || !found {
			slog.Warn("Cached poster not found after store",
				"component", "services", "mediaName", item.MediaName, "cacheKey", cacheKey)
			continue
		}

		// Compose overlay
		overlayData, err := poster.ComposeOverlay(originalData, daysRemaining)
		if err != nil {
			slog.Warn("Failed to compose poster overlay",
				"component", "services", "mediaName", item.MediaName, "error", err)
			s.bus.Publish(events.PosterOverlayFailedEvent{
				MediaName: item.MediaName, IntegrationID: integrationID,
				Error: fmt.Sprintf("compose: %v", err),
			})
			continue
		}

		// Upload overlay
		if err := mgr.UploadPosterImage(nativeID, overlayData, "image/jpeg"); err != nil {
			slog.Warn("Failed to upload poster overlay",
				"component", "services", "mediaName", item.MediaName,
				"integrationID", integrationID, "error", err)
			s.bus.Publish(events.PosterOverlayFailedEvent{
				MediaName: item.MediaName, IntegrationID: integrationID,
				Error: fmt.Sprintf("upload: %v", err),
			})
			continue
		}

		s.bus.Publish(events.PosterOverlayAppliedEvent{
			MediaName: item.MediaName, IntegrationID: integrationID,
			DaysRemaining: daysRemaining,
		})
	}

	// Mark as active
	s.db.Model(&item).Update("poster_overlay_active", true)
	return nil
}

// RestoreOriginal restores the original poster from cache for all media servers.
func (s *PosterOverlayService) RestoreOriginal(item db.SunsetQueueItem, deps PosterDeps) error {
	if deps.Registry == nil || item.TmdbID == nil {
		return nil
	}

	managers := deps.Registry.PosterManagers()

	for integrationID, mgr := range managers {
		// Resolve TMDb ID → native ID for this specific integration
		idMap := deps.TMDbToNativeID[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}

		cacheKey := s.cacheKeyForItem(integrationID, item)
		originalData, found, err := s.cache.Get(cacheKey)
		if err != nil || !found {
			// No cached original — try to restore via the media server's native restore
			if restoreErr := mgr.RestorePosterImage(nativeID); restoreErr != nil {
				slog.Warn("Failed to restore poster via media server",
					"component", "services", "mediaName", item.MediaName,
					"integrationID", integrationID, "error", restoreErr)
			}
			continue
		}

		// Upload the cached original back
		if err := mgr.UploadPosterImage(nativeID, originalData, "image/jpeg"); err != nil {
			slog.Warn("Failed to upload original poster",
				"component", "services", "mediaName", item.MediaName,
				"integrationID", integrationID, "error", err)
			continue
		}

		// Clean up cache
		_ = s.cache.Delete(cacheKey)

		s.bus.Publish(events.PosterOverlayRestoredEvent{
			MediaName: item.MediaName, IntegrationID: integrationID,
		})
	}

	// Mark as inactive
	s.db.Model(&item).Update("poster_overlay_active", false)
	return nil
}

// UpdateAll updates poster overlays for all sunset queue items.
// Called by the daily cron job and the force-refresh route.
// Returns the number of items successfully updated.
func (s *PosterOverlayService) UpdateAll(sunset *SunsetService, deps PosterDeps) (int, error) {
	items, err := sunset.ListAll()
	if err != nil {
		return 0, fmt.Errorf("list sunset items: %w", err)
	}

	updated := 0
	for _, item := range items {
		daysRemaining := sunset.DaysRemaining(item)
		if err := s.UpdateOverlay(item, daysRemaining, deps); err != nil {
			slog.Warn("Failed to update poster overlay",
				"component", "services", "mediaName", item.MediaName, "error", err)
			continue
		}
		updated++
	}

	if updated > 0 {
		slog.Info("Updated poster overlays", "component", "services", "count", updated)
	}
	return updated, nil
}

// RestoreAll restores original posters for all sunset queue items that have
// active overlays. Emergency button.
func (s *PosterOverlayService) RestoreAll(_ *SunsetService, deps PosterDeps) (int, error) {
	var items []db.SunsetQueueItem
	if err := s.db.Where("poster_overlay_active = ?", true).Find(&items).Error; err != nil {
		return 0, fmt.Errorf("list items with active overlays: %w", err)
	}

	restored := 0
	for _, item := range items {
		if err := s.RestoreOriginal(item, deps); err != nil {
			slog.Warn("Failed to restore poster",
				"component", "services", "mediaName", item.MediaName, "error", err)
			continue
		}
		restored++
	}
	return restored, nil
}

// ValidateCache checks that cached originals exist on disk for all items with
// active poster overlays. Logs warnings for missing cache entries.
// Called at startup — does not require the integration registry.
func (s *PosterOverlayService) ValidateCache() {
	var items []db.SunsetQueueItem
	if err := s.db.Where("poster_overlay_active = ?", true).Find(&items).Error; err != nil {
		slog.Warn("Failed to query items for poster cache validation",
			"component", "services", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	// List all cached files and build a lookup set
	cachedKeys, err := s.cache.ListAll()
	if err != nil {
		slog.Warn("Failed to list poster cache directory",
			"component", "services", "error", err)
		return
	}
	cachedSet := make(map[string]bool, len(cachedKeys))
	for _, k := range cachedKeys {
		cachedSet[k] = true
	}

	// Check each active-overlay item. Since we don't know which integration ID
	// was used, we check whether ANY cache key matches this item's TMDb ID.
	// Cache keys are formatted as "{integrationID}_{tmdbID}_orig.jpg".
	missing := 0
	for _, item := range items {
		if item.TmdbID == nil {
			continue
		}
		tmdbStr := fmt.Sprintf("_%d_", *item.TmdbID)
		found := false
		for _, k := range cachedKeys {
			if len(k) > 0 && strings.Contains(k, tmdbStr) {
				found = true
				break
			}
		}
		if !found {
			missing++
			slog.Warn("Poster cache missing for item with active overlay",
				"component", "services", "mediaName", item.MediaName,
				"tmdbId", *item.TmdbID, "cacheDir", s.cache.Dir())
		}
	}

	if missing > 0 {
		slog.Warn("Poster cache validation complete — missing originals detected",
			"component", "services", "activeOverlays", len(items), "missingCache", missing,
			"action", "Use 'Restore All Posters' in settings to re-download originals")
	} else {
		slog.Info("Poster cache validation passed",
			"component", "services", "activeOverlays", len(items))
	}
}

// cacheKeyForItem generates the cache key for a sunset queue item and integration.
func (s *PosterOverlayService) cacheKeyForItem(integrationID uint, item db.SunsetQueueItem) string {
	tmdbID := 0
	if item.TmdbID != nil {
		tmdbID = *item.TmdbID
	}
	return poster.CacheKey(integrationID, tmdbID, "orig")
}
