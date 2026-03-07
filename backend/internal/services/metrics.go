package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"capacitarr/internal/db"
)

// MetricsService consolidates inline DB queries for metrics, history, and
// dashboard statistics. It delegates worker-specific stats to EngineService
// and DeletionService.
type MetricsService struct {
	db       *gorm.DB
	engine   *EngineService
	deletion *DeletionService
}

// NewMetricsService creates a new MetricsService.
func NewMetricsService(database *gorm.DB, engine *EngineService, deletion *DeletionService) *MetricsService {
	return &MetricsService{db: database, engine: engine, deletion: deletion}
}

// GetHistory returns library history entries filtered by resolution, disk group, and time range.
// The since parameter supports: "1h", "24h", "7d", "30d".
func (s *MetricsService) GetHistory(resolution, diskGroupID, since string) ([]db.LibraryHistory, error) {
	if resolution == "" {
		resolution = "raw"
	}

	query := s.db.Where("resolution = ?", resolution)
	if diskGroupID != "" {
		query = query.Where("disk_group_id = ?", diskGroupID)
	}

	// Apply time range filter
	if since != "" {
		var duration time.Duration
		switch since {
		case "1h":
			duration = 1 * time.Hour
		case "24h":
			duration = 24 * time.Hour
		case "7d":
			duration = 7 * 24 * time.Hour
		case "30d":
			duration = 30 * 24 * time.Hour
		}
		if duration > 0 {
			cutoff := time.Now().Add(-duration)
			query = query.Where("timestamp >= ?", cutoff)
		}
	}

	history := make([]db.LibraryHistory, 0)
	if err := query.Order("timestamp asc").Limit(1000).Find(&history).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch metrics history: %w", err)
	}

	return history, nil
}

// GetLifetimeStats returns the singleton lifetime stats row, creating it if it doesn't exist.
func (s *MetricsService) GetLifetimeStats() (db.LifetimeStats, error) {
	var stats db.LifetimeStats
	if err := s.db.FirstOrCreate(&stats, db.LifetimeStats{ID: 1}).Error; err != nil {
		return stats, fmt.Errorf("failed to fetch lifetime stats: %w", err)
	}
	return stats, nil
}

// GetDashboardStats aggregates lifetime stats, protected count, and library
// growth rate into a single response for the dashboard.
func (s *MetricsService) GetDashboardStats() (map[string]interface{}, error) {
	// 1. Lifetime stats
	var lifetime db.LifetimeStats
	if err := s.db.FirstOrCreate(&lifetime, db.LifetimeStats{ID: 1}).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch lifetime stats: %w", err)
	}

	// 2. Protected count from engine service
	engineStats := s.engine.GetStats()
	protectedCount, _ := engineStats["protectedCount"].(int64)

	// 3. Library growth rate: compare most recent entry to 7 days ago
	growthBytes := int64(0)
	hasGrowthData := false

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	// Use Find+Limit instead of First to avoid GORM logging "record not found" —
	// having no library history is expected on fresh installs or after data resets.
	var recentRows []db.LibraryHistory
	s.db.Where("resolution = ?", "raw").
		Order("timestamp DESC").Limit(1).Find(&recentRows)
	if len(recentRows) > 0 {
		recent := recentRows[0]
		var weekAgoRows []db.LibraryHistory
		s.db.Where("resolution = ? AND timestamp <= ?", "raw", cutoff).
			Order("timestamp DESC").Limit(1).Find(&weekAgoRows)
		if len(weekAgoRows) > 0 {
			weekAgo := weekAgoRows[0]
			growthBytes = recent.UsedCapacity - weekAgo.UsedCapacity
			hasGrowthData = true
		}
	}

	return map[string]interface{}{
		"totalBytesReclaimed": lifetime.TotalBytesReclaimed,
		"totalItemsRemoved":   lifetime.TotalItemsRemoved,
		"totalEngineRuns":     lifetime.TotalEngineRuns,
		"protectedCount":      protectedCount,
		"growthBytesPerWeek":  growthBytes,
		"hasGrowthData":       hasGrowthData,
	}, nil
}

// GetWorkerMetrics assembles worker metrics from the EngineService and DeletionService.
// Keys match the frontend TypeScript WorkerStats interface.
func (s *MetricsService) GetWorkerMetrics() map[string]interface{} {
	stats := s.engine.GetStats()

	// Add poll interval from preferences
	var prefs db.PreferenceSet
	if err := s.db.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1}).Error; err == nil {
		stats["pollIntervalSeconds"] = prefs.PollIntervalSeconds
	}

	// Add deletion worker state
	stats["queueDepth"] = 0 // Queue depth is internal to DeletionService
	stats["currentlyDeleting"] = s.deletion.CurrentlyDeleting()
	stats["processed"] = s.deletion.Processed()
	stats["failed"] = s.deletion.Failed()

	return stats
}
