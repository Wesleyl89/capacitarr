// Package jobs manages scheduled background tasks and cron execution.
package jobs

import (
	"log/slog"
	"time"

	"capacitarr/internal/services"

	"github.com/robfig/cron/v3"
)

// Start creates and starts the background cron scheduler for time-series rollup and pruning jobs.
func Start(reg *services.Registry) *cron.Cron {
	c := cron.New()

	// 1. Rollup "raw" to "hourly" every hour at minute 0
	_, err := c.AddFunc("@hourly", func() {
		slog.Info("Running hourly rollup", "component", "jobs")
		start := time.Now().Add(-time.Hour).Truncate(time.Hour)
		end := time.Now().Truncate(time.Hour)
		if rollupErr := reg.Metrics.RollupHistory("raw", "hourly", start, end); rollupErr != nil {
			slog.Error("Hourly rollup failed", "component", "jobs", "error", rollupErr)
		}
		// Keep raw data for 2 hours (enough for real-time zooming)
		if _, pruneErr := reg.Metrics.PruneHistory("raw", time.Now().Add(-2*time.Hour)); pruneErr != nil {
			slog.Error("Failed to prune raw history", "component", "jobs", "error", pruneErr)
		}
	})
	if err != nil {
		slog.Error("Failed to add hourly cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 2. Rollup "hourly" to "daily" every day at midnight
	_, err = c.AddFunc("@daily", func() {
		slog.Info("Running daily rollup", "component", "jobs")
		start := time.Now().Add(-24 * time.Hour).Truncate(24 * time.Hour)
		end := time.Now().Truncate(24 * time.Hour)
		if rollupErr := reg.Metrics.RollupHistory("hourly", "daily", start, end); rollupErr != nil {
			slog.Error("Daily rollup failed", "component", "jobs", "error", rollupErr)
		}
		// Keep hourly snapshots for 7 days
		if _, pruneErr := reg.Metrics.PruneHistory("hourly", time.Now().Add(-7*24*time.Hour)); pruneErr != nil {
			slog.Error("Failed to prune hourly history", "component", "jobs", "error", pruneErr)
		}
	})
	if err != nil {
		slog.Error("Failed to add daily cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 3. Rollup "daily" to "weekly" every week on Sunday at midnight
	_, err = c.AddFunc("@weekly", func() {
		slog.Info("Running weekly rollup", "component", "jobs")
		start := time.Now().Add(-7 * 24 * time.Hour).Truncate(24 * time.Hour)
		end := time.Now().Truncate(24 * time.Hour)
		if rollupErr := reg.Metrics.RollupHistory("daily", "weekly", start, end); rollupErr != nil {
			slog.Error("Weekly rollup failed", "component", "jobs", "error", rollupErr)
		}
		// Keep daily snapshots for 30 days
		if _, pruneErr := reg.Metrics.PruneHistory("daily", time.Now().Add(-30*24*time.Hour)); pruneErr != nil {
			slog.Error("Failed to prune daily history", "component", "jobs", "error", pruneErr)
		}
	})
	if err != nil {
		slog.Error("Failed to add weekly cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 4. Prune "weekly" data older than 1 year
	_, err = c.AddFunc("@monthly", func() {
		slog.Info("Running pruning of old data", "component", "jobs")
		if _, pruneErr := reg.Metrics.PruneHistory("weekly", time.Now().Add(-365*24*time.Hour)); pruneErr != nil {
			slog.Error("Failed to prune weekly history", "component", "jobs", "error", pruneErr)
		}
	})
	if err != nil {
		slog.Error("Failed to add monthly cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 5. Prune old engine run stats — keep the last 1000 rows
	_, err = c.AddFunc("@daily", func() {
		if deleted, pruneErr := reg.Engine.PruneOldStats(1000); pruneErr != nil {
			slog.Error("Failed to prune engine run stats", "component", "jobs", "error", pruneErr)
		} else if deleted > 0 {
			slog.Info("Pruned old engine run stats", "component", "jobs", "deleted", deleted, "kept", 1000)
		}
	})
	if err != nil {
		slog.Error("Failed to add engine stats cleanup cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 6. Prune old in-app notifications — uses audit log retention setting
	_, err = c.AddFunc("@daily", func() {
		prefs, prefsErr := reg.Settings.GetPreferences()
		if prefsErr != nil {
			slog.Error("Failed to fetch preferences for notification cleanup", "component", "jobs", "error", prefsErr)
			return
		}
		if prefs.AuditLogRetentionDays <= 0 {
			return // 0 = keep forever
		}
		cutoff := time.Now().Add(-time.Duration(prefs.AuditLogRetentionDays) * 24 * time.Hour)
		if deleted, pruneErr := reg.NotificationChannel.PruneOldInApp(cutoff); pruneErr != nil {
			slog.Error("Failed to prune old notifications", "component", "jobs", "error", pruneErr)
		} else if deleted > 0 {
			slog.Info("Pruned old in-app notifications", "component", "jobs", "deleted", deleted, "retentionDays", prefs.AuditLogRetentionDays)
		}
	})
	if err != nil {
		slog.Error("Failed to add notification cleanup cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 7. Prune old activity events — fixed 7-day retention
	_, err = c.AddFunc("@daily", func() {
		if deleted, pruneErr := reg.Settings.PruneOldActivities(7); pruneErr != nil {
			slog.Error("Failed to prune activity events", "component", "jobs", "error", pruneErr)
		} else if deleted > 0 {
			slog.Info("Pruned old activity events", "component", "jobs", "deleted", deleted, "retention", "7 days")
		}
	})
	if err != nil {
		slog.Error("Failed to add activity events cleanup cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	// 8. Prune old audit log entries — uses audit log retention setting
	_, err = c.AddFunc("@daily", func() {
		prefs, prefsErr := reg.Settings.GetPreferences()
		if prefsErr != nil {
			slog.Error("Failed to fetch preferences for audit log cleanup", "component", "jobs", "error", prefsErr)
			return
		}
		if deleted, pruneErr := reg.AuditLog.PruneOlderThan(prefs.AuditLogRetentionDays); pruneErr != nil {
			slog.Error("Failed to prune audit log", "component", "jobs", "error", pruneErr)
		} else if deleted > 0 {
			slog.Info("Pruned old audit log entries", "component", "jobs", "deleted", deleted, "retentionDays", prefs.AuditLogRetentionDays)
		}
	})
	if err != nil {
		slog.Error("Failed to add audit log cleanup cron", "component", "jobs", "operation", "add_cron", "error", err)
	}

	c.Start()
	slog.Info("Cron jobs started successfully", "component", "jobs")
	return c
}
