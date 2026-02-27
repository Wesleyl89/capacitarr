package jobs

import (
	"log/slog"
	"time"

	"github.com/capacitarr/capacitarr/backend/internal/db"
	"github.com/robfig/cron/v3"
)

func Start() {
	c := cron.New()

	// 1. Rollup "raw" to "hourly" every hour at minute 0
	_, err := c.AddFunc("@hourly", func() {
		slog.Info("Running hourly rollup")
		rollupData("raw", "hourly", time.Now().Add(-time.Hour).Truncate(time.Hour), time.Now().Truncate(time.Hour))
		// Keep raw data for 2 hours (enough for real-time zooming)
		pruneData("raw", time.Now().Add(-2*time.Hour))
	})
	if err != nil {
		slog.Error("Failed to add hourly cron", "error", err)
	}

	// 2. Rollup "hourly" to "daily" every day at midnight
	_, err = c.AddFunc("@daily", func() {
		slog.Info("Running daily rollup")
		rollupData("hourly", "daily", time.Now().Add(-24*time.Hour).Truncate(24*time.Hour), time.Now().Truncate(24*time.Hour))
		// Keep hourly snapshots for 7 days
		pruneData("hourly", time.Now().Add(-7*24*time.Hour))
	})
	if err != nil {
		slog.Error("Failed to add daily cron", "error", err)
	}

	// 3. Rollup "daily" to "weekly" every week on Sunday at midnight
	_, err = c.AddFunc("@weekly", func() {
		slog.Info("Running weekly rollup")
		rollupData("daily", "weekly", time.Now().Add(-7*24*time.Hour).Truncate(24*time.Hour), time.Now().Truncate(24*time.Hour))
		// Keep daily snapshots for 30 days
		pruneData("daily", time.Now().Add(-30*24*time.Hour))
	})
	if err != nil {
		slog.Error("Failed to add weekly cron", "error", err)
	}

	// 4. Prune "weekly" data older than 1 year
	_, err = c.AddFunc("@monthly", func() {
		slog.Info("Running pruning of old data")
		pruneData("weekly", time.Now().Add(-365*24*time.Hour))
	})
	if err != nil {
		slog.Error("Failed to add monthly cron", "error", err)
	}

	c.Start()
	slog.Info("Cron jobs started successfully")
}

func rollupData(fromRes, toRes string, start, end time.Time) {
	var avgResult struct {
		AvgTotal float64
		AvgUsed  float64
	}

	err := db.DB.Model(&db.LibraryHistory{}).
		Select("AVG(total_capacity) as avg_total, AVG(used_capacity) as avg_used").
		Where("resolution = ? AND timestamp >= ? AND timestamp < ?", fromRes, start, end).
		Scan(&avgResult).Error

	if err != nil {
		slog.Error("Failed to calculate average for rollup", "error", err, "from", fromRes)
		return
	}

	if avgResult.AvgTotal > 0 {
		record := db.LibraryHistory{
			Timestamp:     start,
			TotalCapacity: int64(avgResult.AvgTotal),
			UsedCapacity:  int64(avgResult.AvgUsed),
			Resolution:    toRes,
		}
		if err := db.DB.Create(&record).Error; err != nil {
			slog.Error("Failed to save rollup record", "error", err, "to", toRes)
		}
	}
}

func pruneData(resolution string, before time.Time) {
	if err := db.DB.Where("resolution = ? AND timestamp < ?", resolution, before).Delete(&db.LibraryHistory{}).Error; err != nil {
		slog.Error("Failed to prune data", "error", err, "resolution", resolution)
	}
}
