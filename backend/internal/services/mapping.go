package services

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrMappingNotFound is returned when a TMDb→NativeID mapping does not exist
// in the database for the requested (tmdbID, integrationID) pair.
var ErrMappingNotFound = errors.New("mapping not found")

// MappingService manages persistent TMDb ID → media server native ID mappings.
// Replaces the ephemeral in-memory maps previously built by
// IntegrationRegistry.BuildTMDbToNativeIDMaps(). Mappings are populated during
// engine poll cycles and consumed by PosterOverlayService and SunsetService for
// label and poster operations.
//
// Follows the established service pattern: accepts *gorm.DB and *events.EventBus.
type MappingService struct {
	db  *gorm.DB
	bus *events.EventBus
}

// NewMappingService creates a new mapping service.
func NewMappingService(database *gorm.DB, bus *events.EventBus) *MappingService {
	return &MappingService{db: database, bus: bus}
}

// Resolve returns the native ID for a TMDb ID on a specific media server.
// Returns ErrMappingNotFound if no mapping exists. This is a DB-only lookup
// with no external API calls.
func (s *MappingService) Resolve(tmdbID int, integrationID uint) (string, error) {
	var m db.MediaServerMapping
	err := s.db.Where("tmdb_id = ? AND integration_id = ?", tmdbID, integrationID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrMappingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve mapping: %w", err)
	}
	return m.NativeID, nil
}

// ResolveAll returns native IDs for multiple TMDb IDs on a specific media server.
// Returns a map of tmdbID→nativeID for all found mappings. Missing TMDb IDs are
// silently omitted from the result (not an error). This avoids N+1 queries when
// processing multiple items in a loop.
func (s *MappingService) ResolveAll(tmdbIDs []int, integrationID uint) (map[int]string, error) {
	if len(tmdbIDs) == 0 {
		return make(map[int]string), nil
	}

	var mappings []db.MediaServerMapping
	err := s.db.Where("integration_id = ? AND tmdb_id IN ?", integrationID, tmdbIDs).Find(&mappings).Error
	if err != nil {
		return nil, fmt.Errorf("resolve all mappings: %w", err)
	}

	result := make(map[int]string, len(mappings))
	for _, m := range mappings {
		result[m.TmdbID] = m.NativeID
	}
	return result, nil
}

// GetMapping returns the full mapping record for a TMDb ID on a specific
// integration. Returns ErrMappingNotFound if no mapping exists. Used by
// ResolveWithSearch to retrieve the stored title for search fallback.
func (s *MappingService) GetMapping(tmdbID int, integrationID uint) (db.MediaServerMapping, error) {
	var m db.MediaServerMapping
	err := s.db.Where("tmdb_id = ? AND integration_id = ?", tmdbID, integrationID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return m, ErrMappingNotFound
	}
	if err != nil {
		return m, fmt.Errorf("get mapping: %w", err)
	}
	return m, nil
}

// BulkUpsert inserts or updates mappings from a poll cycle batch. Uses SQLite's
// INSERT OR REPLACE semantics via GORM's OnConflict clause. Updates updated_at
// on every upsert, serving as the "touch" timestamp for Layer 2 freshness
// verification. Processes in batches of 500 to avoid SQLite variable limits.
func (s *MappingService) BulkUpsert(mappings []db.MediaServerMapping) error {
	if len(mappings) == 0 {
		return nil
	}

	// Set updated_at to now for all mappings
	now := time.Now().UTC()
	for i := range mappings {
		mappings[i].UpdatedAt = now
	}

	// Process in batches of 500 (SQLite has a variable limit of 999)
	const batchSize = 500
	for i := 0; i < len(mappings); i += batchSize {
		end := i + batchSize
		if end > len(mappings) {
			end = len(mappings)
		}
		batch := mappings[i:end]

		err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tmdb_id"},
				{Name: "integration_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"native_id", "media_type", "title", "updated_at",
			}),
		}).Create(&batch).Error
		if err != nil {
			return fmt.Errorf("bulk upsert mappings (batch %d-%d): %w", i, end, err)
		}
	}

	slog.Debug("Bulk upserted media server mappings",
		"component", "services", "count", len(mappings))
	return nil
}

// TouchedBefore returns the count of mappings for an integration whose
// updated_at is before the given time. Used for Layer 2 freshness monitoring
// to detect mappings not seen in the current poll cycle.
func (s *MappingService) TouchedBefore(integrationID uint, before time.Time) (int64, error) {
	var count int64
	err := s.db.Model(&db.MediaServerMapping{}).
		Where("integration_id = ? AND updated_at < ?", integrationID, before).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count stale mappings: %w", err)
	}
	return count, nil
}

// DeleteStale removes mappings for an integration whose updated_at is before
// the given time. Returns the number of deleted rows.
func (s *MappingService) DeleteStale(integrationID uint, before time.Time) (int64, error) {
	result := s.db.Where("integration_id = ? AND updated_at < ?", integrationID, before).
		Delete(&db.MediaServerMapping{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete stale mappings: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// GarbageCollect removes stale and orphaned mappings. Deletes:
// 1. Mappings older than maxAge (item likely removed from media server).
// 2. Mappings for integrations that no longer exist (orphaned by deletion).
// Returns the total number of deleted rows.
func (s *MappingService) GarbageCollect(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-maxAge)
	var totalDeleted int64

	// 1. Delete mappings older than maxAge for enabled integrations
	result := s.db.Where("updated_at < ?", cutoff).Delete(&db.MediaServerMapping{})
	if result.Error != nil {
		return 0, fmt.Errorf("gc stale mappings: %w", result.Error)
	}
	totalDeleted += result.RowsAffected

	// 2. Delete orphaned mappings (integration no longer exists)
	// The ON DELETE CASCADE FK should handle this, but belt-and-suspenders
	// for cases where FK enforcement is disabled or the cascade didn't fire.
	result = s.db.Where("integration_id NOT IN (?)",
		s.db.Model(&db.IntegrationConfig{}).Select("id"),
	).Delete(&db.MediaServerMapping{})
	if result.Error != nil {
		return totalDeleted, fmt.Errorf("gc orphaned mappings: %w", result.Error)
	}
	totalDeleted += result.RowsAffected

	if totalDeleted > 0 {
		slog.Info("Garbage collected media server mappings",
			"component", "services", "stale", result.RowsAffected, "total", totalDeleted)
	}
	return totalDeleted, nil
}

// Invalidate deletes the mapping for a specific (tmdbID, integrationID) pair.
// Used by Layer 1 (passive 404 verification) when a native ID is detected as stale.
func (s *MappingService) Invalidate(tmdbID int, integrationID uint) error {
	result := s.db.Where("tmdb_id = ? AND integration_id = ?", tmdbID, integrationID).
		Delete(&db.MediaServerMapping{})
	if result.Error != nil {
		return fmt.Errorf("invalidate mapping: %w", result.Error)
	}
	return nil
}

// ResolveWithSearch attempts a DB lookup first. On miss, falls back to a
// targeted search against the media server via NativeIDSearcher. If the
// search succeeds, the result is stored in the mapping table and returned.
// This eliminates the regression window where newly-added items can't be
// resolved between poll cycles.
func (s *MappingService) ResolveWithSearch(tmdbID int, integrationID uint, title string, searcher integrations.NativeIDSearcher) (string, error) {
	// Try DB first (fast path)
	nativeID, err := s.Resolve(tmdbID, integrationID)
	if err == nil {
		return nativeID, nil
	}
	if !errors.Is(err, ErrMappingNotFound) {
		return "", err
	}

	// DB miss — fall through to targeted search
	if searcher == nil || title == "" {
		return "", ErrMappingNotFound
	}

	nativeID, searchErr := searcher.SearchByTMDbID(title, tmdbID)
	if searchErr != nil {
		return "", ErrMappingNotFound
	}

	// Store the discovered mapping for future lookups
	mapping := db.MediaServerMapping{
		TmdbID:        tmdbID,
		IntegrationID: integrationID,
		NativeID:      nativeID,
		Title:         title,
	}
	if upsertErr := s.BulkUpsert([]db.MediaServerMapping{mapping}); upsertErr != nil {
		slog.Warn("Failed to store search-discovered mapping",
			"component", "services", "tmdbID", tmdbID, "integrationID", integrationID, "error", upsertErr)
	}

	return nativeID, nil
}

// InvalidateAndResolve deletes a stale mapping and attempts to re-discover
// the native ID via targeted search. Used by Layer 1 (passive 404 verification)
// when a poster upload or label operation gets a 404 from the media server.
func (s *MappingService) InvalidateAndResolve(tmdbID int, integrationID uint, title string, searcher integrations.NativeIDSearcher) (string, error) {
	// Delete the stale mapping
	if err := s.Invalidate(tmdbID, integrationID); err != nil {
		slog.Warn("Failed to invalidate stale mapping",
			"component", "services", "tmdbID", tmdbID, "integrationID", integrationID, "error", err)
	}

	if searcher == nil || title == "" {
		return "", ErrMappingNotFound
	}

	// Attempt targeted search
	nativeID, searchErr := searcher.SearchByTMDbID(title, tmdbID)
	if searchErr != nil {
		return "", ErrMappingNotFound
	}

	// Store the new mapping
	mapping := db.MediaServerMapping{
		TmdbID:        tmdbID,
		IntegrationID: integrationID,
		NativeID:      nativeID,
		Title:         title,
	}
	if upsertErr := s.BulkUpsert([]db.MediaServerMapping{mapping}); upsertErr != nil {
		slog.Warn("Failed to store re-discovered mapping",
			"component", "services", "tmdbID", tmdbID, "integrationID", integrationID, "error", upsertErr)
	}

	return nativeID, nil
}
