package poller

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
	"capacitarr/internal/services"

	"gorm.io/gorm"
)

// Poller orchestrates periodic media library polling and capacity evaluation.
// All state is on the struct — no package-level globals.
type Poller struct {
	reg  *services.Registry
	done chan struct{}

	// Per-run metrics (reset each engine cycle, synced to EngineService at the end)
	lastRunEvaluated int64
	lastRunFlagged   int64
	lastRunProtected int64
}

// New creates a new Poller bound to the given service registry.
func New(reg *services.Registry) *Poller {
	return &Poller{
		reg:  reg,
		done: make(chan struct{}),
	}
}

// Start begins the continuous polling loop. Call Stop() to terminate.
func (p *Poller) Start() {
	go func() {
		timer := time.NewTimer(p.getPollInterval())
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				p.safePoll()
				timer.Reset(p.getPollInterval())
			case <-p.reg.Engine.RunNowCh:
				slog.Info("Manual run triggered via API", "component", "poller")
				p.safePoll()
				// Don't reset the timer — let the next scheduled tick proceed normally
			case <-p.done:
				return
			}
		}
	}()
}

// Stop signals the poller goroutine to exit.
func (p *Poller) Stop() {
	close(p.done)
}

// getPollInterval reads PollIntervalSeconds from the database preference set.
// Falls back to 300s (5 min) if not set, and enforces a 30s minimum.
func (p *Poller) getPollInterval() time.Duration {
	var prefs db.PreferenceSet
	if err := p.reg.DB.First(&prefs, 1).Error; err != nil {
		return 5 * time.Minute
	}
	secs := prefs.PollIntervalSeconds
	if secs < 30 {
		secs = 300
	}
	return time.Duration(secs) * time.Second
}

// safePoll wraps poll() with panic recovery so a single failing cycle
// doesn't crash the entire poller goroutine.
func (p *Poller) safePoll() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panic recovered in poll cycle", "component", "poller", "panic", r)
			p.reg.Engine.SetRunning(false) // ensure the lock is released
		}
	}()
	p.poll()
}

func (p *Poller) poll() {
	if p.reg.Engine.IsRunning() {
		slog.Info("Skipping poll — previous run still in progress", "component", "poller")
		return
	}
	p.reg.Engine.SetRunning(true)
	defer p.reg.Engine.SetRunning(false)

	database := p.reg.DB
	bus := p.reg.Bus

	pollStart := time.Now()

	// Clean expired snoozes at the start of each cycle — resets rejected items
	// with expired snoozed_until back to pending so they're re-evaluated.
	if count, err := p.reg.Approval.CleanExpiredSnoozes(); err != nil {
		slog.Error("Failed to clean expired snoozes", "component", "poller", "error", err)
	} else if count > 0 {
		slog.Info("Cleaned expired snoozes at cycle start", "component", "poller", "count", count)
	}

	// Increment lifetime engine runs counter (atomic DB update)
	database.Model(&db.LifetimeStats{}).Where("id = 1").
		UpdateColumn("total_engine_runs", gorm.Expr("total_engine_runs + ?", 1))

	// Reset per-run counters at the start of each poll cycle
	atomic.StoreInt64(&p.lastRunEvaluated, 0)
	atomic.StoreInt64(&p.lastRunFlagged, 0)
	atomic.StoreInt64(&p.lastRunProtected, 0)

	var configs []db.IntegrationConfig
	if err := database.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		slog.Error("Failed to load integrations", "component", "poller", "operation", "load_integrations", "error", err)
		bus.Publish(events.EngineErrorEvent{Error: fmt.Sprintf("failed to load integrations: %v", err)})
		return
	}

	var prefs db.PreferenceSet
	if err := database.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1}).Error; err != nil {
		slog.Error("Failed to load preferences", "component", "poller", "operation", "load_preferences", "error", err)
		return
	}

	// Create engine run stats row (zeroed counters, will be updated after evaluation)
	runStats := db.EngineRunStats{
		RunAt:         pollStart,
		ExecutionMode: prefs.ExecutionMode,
	}
	if err := database.Create(&runStats).Error; err != nil {
		slog.Error("Failed to create engine run stats", "component", "poller", "operation", "create_stats", "error", err)
	}

	// Publish engine start event
	bus.Publish(events.EngineStartEvent{ExecutionMode: prefs.ExecutionMode})

	slog.Debug("Poll cycle starting", "component", "poller",
		"enabledIntegrations", len(configs),
		"pollInterval", prefs.PollIntervalSeconds,
		"executionMode", prefs.ExecutionMode)

	if len(configs) == 0 {
		slog.Debug("No enabled integrations, skipping poll", "component", "poller")
		return
	}

	// Fetch media items, disk space, and enrichment clients from all integrations
	fetched := fetchAllIntegrations(configs, database)

	// Enrich items with watch history and request data
	enrichItems(fetched.allItems, fetched.enrichment)

	// Find the most specific mount for each root folder
	mediaMounts := findMediaMounts(fetched.diskMap, fetched.rootFolders)

	// Update DiskGroups and record history only for media mounts
	for mountPath := range mediaMounts {
		disk := fetched.diskMap[mountPath]
		usedBytes := disk.TotalBytes - disk.FreeBytes

		// Upsert DiskGroup
		var group db.DiskGroup
		result := database.Where("mount_path = ?", mountPath).First(&group)
		if result.Error != nil {
			group = db.DiskGroup{
				MountPath:  mountPath,
				TotalBytes: disk.TotalBytes,
				UsedBytes:  usedBytes,
			}
			database.Create(&group)
		} else {
			database.Model(&group).Updates(map[string]interface{}{
				"total_bytes": disk.TotalBytes,
				"used_bytes":  usedBytes,
			})
			// Update the local struct values for threshold check
			group.TotalBytes = disk.TotalBytes
			group.UsedBytes = usedBytes
		}

		// Record LibraryHistory snapshot
		record := db.LibraryHistory{
			Timestamp:     time.Now(),
			TotalCapacity: disk.TotalBytes,
			UsedCapacity:  usedBytes,
			Resolution:    "raw",
			DiskGroupID:   &group.ID,
		}
		if err := database.Create(&record).Error; err != nil {
			slog.Error("Failed to save capacity record", "component", "poller", "operation", "save_capacity",
				"mount", mountPath, "error", err)
		}

		// Evaluate and trigger cleanup if threshold breached
		p.evaluateAndCleanDisk(group, fetched.allItems, fetched.serviceClients, runStats.ID)
	}

	// Clean up orphaned disk groups that are no longer media mounts
	if len(mediaMounts) > 0 {
		var allGroups []db.DiskGroup
		if err := database.Find(&allGroups).Error; err != nil {
			slog.Error("Failed to fetch disk groups for orphan cleanup", "component", "poller", "error", err)
		}
		for _, g := range allGroups {
			if !mediaMounts[g.MountPath] {
				slog.Info("Removing orphaned disk group", "component", "poller",
					"mount", g.MountPath, "id", g.ID)
				database.Where("disk_group_id = ?", g.ID).Delete(&db.LibraryHistory{})
				database.Delete(&g)
			}
		}
	}

	// Update engine run stats with final counters.
	// Note: freed_bytes is NOT set here — it is incremented by the deletion worker
	// after each successful DeleteMediaItem() call, ensuring it only reflects actual
	// bytes freed (not flagged/queued bytes that were never deleted).
	evaluated := atomic.LoadInt64(&p.lastRunEvaluated)
	flagged := atomic.LoadInt64(&p.lastRunFlagged)
	protected := atomic.LoadInt64(&p.lastRunProtected)

	database.Model(&db.EngineRunStats{}).Where("id = ?", runStats.ID).Updates(map[string]interface{}{
		"evaluated":   int(evaluated),
		"flagged":     int(flagged),
		"duration_ms": time.Since(pollStart).Milliseconds(),
	})

	// Sync per-run stats to EngineService for API consumers
	p.reg.Engine.SetLastRunStats(int(evaluated), int(flagged), int(protected))

	// Publish engine complete event
	bus.Publish(events.EngineCompleteEvent{
		Evaluated:     int(evaluated),
		Flagged:       int(flagged),
		DurationMs:    time.Since(pollStart).Milliseconds(),
		ExecutionMode: prefs.ExecutionMode,
	})

	slog.Debug("Poll cycle complete", "component", "poller",
		"duration", time.Since(pollStart).String(),
		"totalItems", len(fetched.allItems),
		"evaluated", evaluated,
		"flagged", flagged,
		"protected", protected)
}

// findMediaMounts returns only the mount paths that are the most specific match
// for at least one root folder. For example, if mounts are ["/", "/media"] and
// root folder is "/media/movies", only "/media" is returned (not "/").
func findMediaMounts(diskMap map[string]integrations.DiskSpace, rootFolders map[string]bool) map[string]bool {
	mediaMounts := make(map[string]bool)

	for rf := range rootFolders {
		cleanRF := strings.TrimRight(rf, "/")
		bestMount := ""
		bestLen := 0

		for mountPath := range diskMap {
			cleanMount := strings.TrimRight(mountPath, "/")
			// Special case: root "/" matches everything
			if cleanMount == "" {
				if bestLen == 0 {
					bestMount = mountPath
				}
				continue
			}
			// Check if root folder lives under this mount
			if strings.HasPrefix(cleanRF, cleanMount+"/") || cleanRF == cleanMount {
				if len(cleanMount) > bestLen {
					bestLen = len(cleanMount)
					bestMount = mountPath
				}
			}
		}

		if bestMount != "" {
			mediaMounts[bestMount] = true
			slog.Debug("Matched root folder to mount", "component", "poller",
				"rootFolder", rf, "mount", bestMount)
		}
	}

	// If we have both "/" and other more specific mounts, drop "/"
	// This handles Docker/container scenarios where different services
	// see different mount namespaces for the same underlying storage
	if len(mediaMounts) > 1 {
		for m := range mediaMounts {
			if strings.TrimRight(m, "/") == "" {
				slog.Debug("Dropping root mount '/' since more specific mounts exist", "component", "poller")
				delete(mediaMounts, m)
			}
		}
	}

	return mediaMounts
}
