package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

// UpdateOverlay downloads the clean original poster from the canonical source
// (the *arr PosterURL, typically TMDb CDN), caches it, composites the countdown
// overlay, and uploads the result to all enabled media servers.
//
// The media server is WRITE-ONLY for posters — we never download from it.
// This prevents overlay stacking (downloading our own overlaid poster and
// overlaying again) and ensures the cached original is always clean.
func (s *PosterOverlayService) UpdateOverlay(item db.SunsetQueueItem, daysRemaining int, deps PosterDeps) error {
	if deps.Registry == nil || item.TmdbID == nil {
		return nil
	}

	// ── Obtain the clean original poster ─────────────────────────────────
	cacheKey := s.cacheKeyForCanonical(item)
	originalData, found, cacheErr := s.cache.Get(cacheKey)
	if !found || cacheErr != nil {
		// Download from the canonical source (*arr PosterURL → TMDb CDN)
		if item.PosterURL == "" {
			slog.Error("No poster URL available for overlay — item has no PosterURL from *arr",
				"component", "services", "mediaName", item.MediaName)
			return nil
		}

		downloaded, dlErr := downloadPoster(item.PosterURL)
		if dlErr != nil {
			slog.Error("Failed to download canonical poster for overlay",
				"component", "services", "mediaName", item.MediaName,
				"url", item.PosterURL, "error", dlErr)
			return nil
		}

		if err := s.cache.Store(cacheKey, downloaded); err != nil {
			slog.Error("Failed to cache original poster",
				"component", "services", "mediaName", item.MediaName, "error", err)
			return nil
		}
		originalData = downloaded
	}

	// ── Compose overlay ──────────────────────────────────────────────────
	overlayData, composeErr := poster.ComposeOverlay(originalData, daysRemaining)
	if composeErr != nil {
		slog.Error("Failed to compose poster overlay",
			"component", "services", "mediaName", item.MediaName, "error", composeErr)
		return nil
	}

	// ── Upload to all media servers (write-only) ─────────────────────────
	managers := deps.Registry.PosterManagers()
	for integrationID, mgr := range managers {
		idMap := deps.TMDbToNativeID[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}

		if err := mgr.UploadPosterImage(nativeID, overlayData, "image/jpeg"); err != nil {
			slog.Error("Failed to upload poster overlay",
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

	s.db.Model(&item).Update("poster_overlay_active", true)
	return nil
}

// RestoreOriginal restores the original poster from the canonical cache for all
// media servers. If no canonical cache exists, re-downloads from the *arr PosterURL.
// Falls back to the media server's native RestorePosterImage as a last resort.
func (s *PosterOverlayService) RestoreOriginal(item db.SunsetQueueItem, deps PosterDeps) error {
	if deps.Registry == nil || item.TmdbID == nil {
		return nil
	}

	// Get the clean original from canonical cache or re-download
	cacheKey := s.cacheKeyForCanonical(item)
	originalData, found, cacheErr := s.cache.Get(cacheKey)
	if !found || cacheErr != nil {
		// Try re-downloading from the *arr PosterURL
		if item.PosterURL != "" {
			downloaded, dlErr := downloadPoster(item.PosterURL)
			if dlErr == nil {
				originalData = downloaded
				found = true
			} else {
				slog.Error("Failed to re-download canonical poster for restore",
					"component", "services", "mediaName", item.MediaName, "error", dlErr)
			}
		}
	}

	managers := deps.Registry.PosterManagers()
	for integrationID, mgr := range managers {
		idMap := deps.TMDbToNativeID[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}

		if found && originalData != nil {
			// Upload the clean original back
			if err := mgr.UploadPosterImage(nativeID, originalData, "image/jpeg"); err != nil {
				slog.Error("Failed to upload original poster for restore",
					"component", "services", "mediaName", item.MediaName,
					"integrationID", integrationID, "error", err)
				continue
			}
		} else {
			// Last resort: use the media server's native restore (unlocks poster)
			if restoreErr := mgr.RestorePosterImage(nativeID); restoreErr != nil {
				slog.Error("Failed to restore poster via media server native restore",
					"component", "services", "mediaName", item.MediaName,
					"integrationID", integrationID, "error", restoreErr)
			}
			continue
		}

		s.bus.Publish(events.PosterOverlayRestoredEvent{
			MediaName: item.MediaName, IntegrationID: integrationID,
		})
	}

	// Clean up cache and mark inactive
	_ = s.cache.Delete(cacheKey)
	s.db.Model(&item).Update("poster_overlay_active", false)
	return nil
}

// UpdateSavedOverlay replaces the countdown overlay with a green "Saved by popular
// demand" banner. Uses the canonical cached original — never downloads from the
// media server. Called when a sunset item is saved due to score drop.
func (s *PosterOverlayService) UpdateSavedOverlay(item db.SunsetQueueItem, deps PosterDeps) error {
	if deps.Registry == nil || item.TmdbID == nil {
		return nil
	}

	// Get the clean original from canonical cache or re-download
	cacheKey := s.cacheKeyForCanonical(item)
	originalData, found, cacheErr := s.cache.Get(cacheKey)
	if !found || cacheErr != nil {
		if item.PosterURL == "" {
			slog.Error("No poster URL available for saved overlay",
				"component", "services", "mediaName", item.MediaName)
			return nil
		}
		downloaded, dlErr := downloadPoster(item.PosterURL)
		if dlErr != nil {
			slog.Error("Failed to download canonical poster for saved overlay",
				"component", "services", "mediaName", item.MediaName, "error", dlErr)
			return nil
		}
		if err := s.cache.Store(cacheKey, downloaded); err != nil {
			slog.Error("Failed to cache original poster",
				"component", "services", "mediaName", item.MediaName, "error", err)
		}
		originalData = downloaded
	}

	savedOverlay, composeErr := poster.ComposeSavedOverlay(originalData)
	if composeErr != nil {
		slog.Error("Failed to compose saved poster overlay",
			"component", "services", "mediaName", item.MediaName, "error", composeErr)
		return nil
	}

	managers := deps.Registry.PosterManagers()
	for integrationID, mgr := range managers {
		idMap := deps.TMDbToNativeID[integrationID]
		if idMap == nil {
			continue
		}
		nativeID, ok := idMap[*item.TmdbID]
		if !ok {
			continue
		}

		if uploadErr := mgr.UploadPosterImage(nativeID, savedOverlay, "image/jpeg"); uploadErr != nil {
			slog.Error("Failed to upload saved poster overlay",
				"component", "services", "mediaName", item.MediaName,
				"integrationID", integrationID, "error", uploadErr)
			continue
		}
	}
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
			slog.Error("Failed to update poster overlay",
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
			slog.Error("Failed to restore poster",
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
		slog.Error("Failed to query items for poster cache validation",
			"component", "services", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	// List all cached files and build a lookup set
	cachedKeys, err := s.cache.ListAll()
	if err != nil {
		slog.Error("Failed to list poster cache directory",
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
// Retained for backward compatibility with RestoreOriginal which may still have
// per-integration cached originals from before the canonical source migration.
func (s *PosterOverlayService) cacheKeyForItem(integrationID uint, item db.SunsetQueueItem) string {
	tmdbID := 0
	if item.TmdbID != nil {
		tmdbID = *item.TmdbID
	}
	return poster.CacheKey(integrationID, tmdbID, "orig")
}

// cacheKeyForCanonical generates a cache key for the canonical (TMDb CDN)
// original poster. Not tied to any specific media server integration.
func (s *PosterOverlayService) cacheKeyForCanonical(item db.SunsetQueueItem) string {
	tmdbID := 0
	if item.TmdbID != nil {
		tmdbID = *item.TmdbID
	}
	return poster.CacheKey(0, tmdbID, "canonical")
}

// downloadPoster fetches a poster image from a URL (typically TMDb CDN).
func downloadPoster(posterURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, posterURL, nil) //nolint:gosec // G107: URL is from *arr metadata, not user input
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, posterURL)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MiB max
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return data, nil
}
