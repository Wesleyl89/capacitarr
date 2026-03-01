package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"capacitarr/internal/db"
	"capacitarr/internal/engine"
	"capacitarr/internal/integrations"
)

// RunNowCh allows triggering an immediate engine evaluation cycle from the API.
var RunNowCh = make(chan struct{}, 1)

// pollRunning prevents concurrent poll() executions when a manual "Run Now"
// overlaps with a ticker-triggered poll.
var pollRunning atomic.Bool

// Start begins the continuous polling loop and returns a stop function.
// It queries all enabled integrations, fetches disk space for media root folders only,
// updates DiskGroups, and records a LibraryHistory snapshot per disk group.
func Start(interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				poll()
			case <-RunNowCh:
				slog.Info("Poller: manual run triggered via API")
				poll()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	return func() {
		close(done)
	}
}

// StopWorker closes the delete queue channel so the deletion worker can drain and exit.
func StopWorker() {
	close(deleteQueue)
}

func poll() {
	if !pollRunning.CompareAndSwap(false, true) {
		slog.Info("Poller: skipping — previous run still in progress")
		return
	}
	defer pollRunning.Store(false)

	var configs []db.IntegrationConfig
	if err := db.DB.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		slog.Error("Poller: failed to load integrations", "error", err)
		return
	}

	var prefs db.PreferenceSet
	db.DB.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1})

	// Prune old audit logs
	if prefs.AuditLogRetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -prefs.AuditLogRetentionDays)
		if err := db.DB.Where("created_at < ?", cutoff).Delete(&db.AuditLog{}).Error; err != nil {
			slog.Error("Poller: failed to prune old audit logs", "error", err)
		}
	}

	if len(configs) == 0 {
		return
	}

	// Collect root folder paths and disk space from *arr integrations
	rootFolders := make(map[string]bool)               // set of root folder paths
	diskMap := make(map[string]integrations.DiskSpace) // all disk entries from *arr

	var allItems []integrations.MediaItem
	serviceClients := make(map[uint]integrations.Integration)

	// Track enrichment-only clients separately (not full Integration implementations)
	var tautulliClient *integrations.TautulliClient
	var overseerrClient *integrations.OverseerrClient

	for _, cfg := range configs {
		// Tautulli is an enrichment-only service, not a full Integration
		if cfg.Type == "tautulli" {
			tautulliClient = integrations.NewTautulliClient(cfg.URL, cfg.APIKey)
			now := time.Now()
			if err := tautulliClient.TestConnection(); err != nil {
				slog.Warn("Poller: Tautulli connection failed", "error", err)
				db.DB.Model(&cfg).Updates(map[string]interface{}{
					"last_error": err.Error(),
				})
			} else {
				db.DB.Model(&cfg).Updates(map[string]interface{}{
					"last_sync":  &now,
					"last_error": "",
				})
			}
			continue
		}

		// Overseerr is an enrichment-only service for tracking media requests
		if cfg.Type == "overseerr" {
			overseerrClient = integrations.NewOverseerrClient(cfg.URL, cfg.APIKey)
			now := time.Now()
			if err := overseerrClient.TestConnection(); err != nil {
				slog.Warn("Poller: Overseerr connection failed", "error", err)
				db.DB.Model(&cfg).Updates(map[string]interface{}{
					"last_error": err.Error(),
				})
			} else {
				db.DB.Model(&cfg).Updates(map[string]interface{}{
					"last_sync":  &now,
					"last_error": "",
				})
			}
			continue
		}

		client := createClient(cfg.Type, cfg.URL, cfg.APIKey)
		if client == nil {
			continue
		}
		serviceClients[cfg.ID] = client

		if cfg.Type == "plex" {
			// Plex is only used for protection rules, not disk usage tracking
			now := time.Now()
			db.DB.Model(&cfg).Updates(map[string]interface{}{
				"last_sync":  &now,
				"last_error": "",
			})
			continue
		}

		// Fetch media items for per-integration usage tracking (Sonarr/Radarr only)
		items, err := client.GetMediaItems()
		if err != nil {
			slog.Warn("Poller: media items fetch failed",
				"integration", cfg.Name, "type", cfg.Type, "error", err)
		} else {
			for i := range items {
				items[i].IntegrationID = cfg.ID
			}
			allItems = append(allItems, items...)

			var totalSize int64
			// For Sonarr, only count show-level items to avoid double-counting seasons
			for _, item := range items {
				if cfg.Type == "sonarr" && item.Type != integrations.MediaTypeShow {
					continue
				}
				totalSize += item.SizeBytes
			}
			mediaCount := len(items)
			if cfg.Type == "sonarr" {
				// Count unique shows only
				mediaCount = 0
				for _, item := range items {
					if item.Type == integrations.MediaTypeShow {
						mediaCount++
					}
				}
			}
			db.DB.Model(&cfg).Updates(map[string]interface{}{
				"media_size_bytes": totalSize,
				"media_count":      mediaCount,
			})
		}

		// Get root folders (Sonarr/Radarr only)
		folders, err := client.GetRootFolders()
		if err != nil {
			slog.Warn("Poller: root folder fetch failed",
				"integration", cfg.Name, "type", cfg.Type, "error", err)
		}
		for _, f := range folders {
			rootFolders[f] = true
			slog.Info("Poller: root folder found",
				"integration", cfg.Name, "path", f)
		}

		// Get disk space
		disks, err := client.GetDiskSpace()
		if err != nil {
			slog.Warn("Poller: disk space fetch failed",
				"integration", cfg.Name, "type", cfg.Type, "error", err)
			db.DB.Model(&cfg).Updates(map[string]interface{}{
				"last_error": err.Error(),
			})
			continue
		}

		// Update last sync time, clear error
		now := time.Now()
		db.DB.Model(&cfg).Updates(map[string]interface{}{
			"last_sync":  &now,
			"last_error": "",
		})

		// Collect all disk entries
		for _, d := range disks {
			if d.Path == "" {
				continue
			}
			if existing, ok := diskMap[d.Path]; ok {
				if d.TotalBytes > existing.TotalBytes {
					diskMap[d.Path] = d
				}
			} else {
				diskMap[d.Path] = d
			}
		}
	}

	// Find the most specific mount for each root folder
	mediaMounts := findMediaMounts(diskMap, rootFolders)

	// Update DiskGroups and record history only for media mounts
	for mountPath := range mediaMounts {
		disk := diskMap[mountPath]
		usedBytes := disk.TotalBytes - disk.FreeBytes

		// Upsert DiskGroup
		var group db.DiskGroup
		result := db.DB.Where("mount_path = ?", mountPath).First(&group)
		if result.Error != nil {
			group = db.DiskGroup{
				MountPath:  mountPath,
				TotalBytes: disk.TotalBytes,
				UsedBytes:  usedBytes,
			}
			db.DB.Create(&group)
		} else {
			db.DB.Model(&group).Updates(map[string]interface{}{
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
		if err := db.DB.Create(&record).Error; err != nil {
			slog.Error("Poller: failed to save capacity record",
				"mount", mountPath, "error", err)
		}

		// Evaluate and trigger cleanup if threshold breached
		evaluateAndCleanDisk(group, allItems, serviceClients)
	}

	// Clean up orphaned disk groups that are no longer media mounts
	if len(mediaMounts) > 0 {
		var allGroups []db.DiskGroup
		db.DB.Find(&allGroups)
		for _, g := range allGroups {
			if !mediaMounts[g.MountPath] {
				slog.Info("Poller: removing orphaned disk group",
					"mount", g.MountPath, "id", g.ID)
				db.DB.Where("disk_group_id = ?", g.ID).Delete(&db.LibraryHistory{})
				db.DB.Delete(&g)
			}
		}
	}
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
			slog.Info("Poller: matched root folder to mount",
				"root_folder", rf, "mount", bestMount)
		}
	}

	// If we have both "/" and other more specific mounts, drop "/"
	// This handles Docker/container scenarios where different services
	// see different mount namespaces for the same underlying storage
	if len(mediaMounts) > 1 {
		for m := range mediaMounts {
			if strings.TrimRight(m, "/") == "" {
				slog.Info("Poller: dropping root mount '/' since more specific mounts exist")
				delete(mediaMounts, m)
			}
		}
	}

	return mediaMounts
}

func createClient(intType, url, apiKey string) integrations.Integration {
	switch intType {
	case "sonarr":
		return integrations.NewSonarrClient(url, apiKey)
	case "radarr":
		return integrations.NewRadarrClient(url, apiKey)
	case "lidarr":
		return integrations.NewLidarrClient(url, apiKey)
	case "plex":
		return integrations.NewPlexClient(url, apiKey)
	default:
		return nil
	}
}

type deleteJob struct {
	client  integrations.Integration
	item    integrations.MediaItem
	reason  string
	score   float64
	factors []engine.ScoreFactor
}

var deleteQueue = make(chan deleteJob, 500)

var (
	metricsProcessed    int64
	metricsFailed       int64
	metricsEvaluated    int64
	metricsActioned     int64
	metricsFreedBytes   int64
	metricsLastRunEpoch int64

	// Per-run metrics (reset each engine evaluation cycle)
	lastRunEvaluated int64
	lastRunFlagged   int64
	lastRunFreedBytes int64

	// Currently-deleting item name (atomic.Value storing string)
	currentlyDeletingVal atomic.Value
)

// GetWorkerMetrics returns the current state of the backend deletion worker
func GetWorkerMetrics() map[string]interface{} {
	var prefs db.PreferenceSet
	db.DB.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1})

	mode := prefs.ExecutionMode
	if mode == "" {
		mode = "dry_run"
	}

	// Safely load currentlyDeleting (may be nil on first access)
	currentlyDeletion := ""
	if v := currentlyDeletingVal.Load(); v != nil {
		currentlyDeletion = v.(string)
	}

	return map[string]interface{}{
		"executionMode":     mode,
		"queueDepth":       len(deleteQueue),
		"lastRunEvaluated":  atomic.LoadInt64(&lastRunEvaluated),
		"lastRunFlagged":    atomic.LoadInt64(&lastRunFlagged),
		"lastRunFreedBytes": atomic.LoadInt64(&lastRunFreedBytes),
		"lastRunEpoch":      atomic.LoadInt64(&metricsLastRunEpoch),
		"currentlyDeleting": currentlyDeletion,
		// Cumulative totals
		"evaluated":  atomic.LoadInt64(&metricsEvaluated),
		"actioned":   atomic.LoadInt64(&metricsActioned),
		"freedBytes": atomic.LoadInt64(&metricsFreedBytes),
		"processed":  atomic.LoadInt64(&metricsProcessed),
		"failed":     atomic.LoadInt64(&metricsFailed),
	}
}

// init starts the background deletion worker before anything else
func init() {
	go deletionWorker()
}

func deletionWorker() {
	// Rate limit: 1 deletion every 3 seconds to protect disk I/O, burst of 1.
	// This is much smarter than arbitrary sleeps, as it smooths out load dynamically.
	limiter := rate.NewLimiter(rate.Every(3*time.Second), 1)

	for job := range deleteQueue {
		// Wait blocks until a token is available
		_ = limiter.Wait(context.Background())

		currentlyDeletingVal.Store(job.item.Title)

		if err := job.client.DeleteMediaItem(job.item); err != nil {
			slog.Error("Background deletion failed", "item", job.item.Title, "error", err)
			atomic.AddInt64(&metricsFailed, 1)
			currentlyDeletingVal.Store("")
			continue
		}

		currentlyDeletingVal.Store("")
		atomic.AddInt64(&metricsProcessed, 1)

		// Log the successful deletion to the audit table
		factorsJSON, _ := json.Marshal(job.factors)
		logEntry := db.AuditLog{
			MediaName:    job.item.Title,
			MediaType:    string(job.item.Type),
			Reason:       fmt.Sprintf("Score: %.2f (%s)", job.score, job.reason),
			ScoreDetails: string(factorsJSON),
			Action:       "Deleted",
			SizeBytes:    job.item.SizeBytes,
			CreatedAt:    time.Now(),
		}
		db.DB.Create(&logEntry)

		slog.Info("Background engine action completed", "media", job.item.Title, "action", "Deleted", "freed", job.item.SizeBytes)
	}
}

func evaluateAndCleanDisk(group db.DiskGroup, allItems []integrations.MediaItem, serviceClients map[uint]integrations.Integration) {
	var prefs db.PreferenceSet
	db.DB.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1})

	currentPct := float64(group.UsedBytes) / float64(group.TotalBytes) * 100
	if currentPct < group.ThresholdPct {
		return
	}

	slog.Info("Disk threshold breached, evaluating media for deletion", "mount", group.MountPath, "currentPct", currentPct, "threshold", group.ThresholdPct)

	// Filter items on this mount
	var diskItems []integrations.MediaItem
	for _, item := range allItems {
		if strings.HasPrefix(item.Path, group.MountPath) {
			diskItems = append(diskItems, item)
		}
	}

	var rules []db.ProtectionRule
	db.DB.Find(&rules)

	// Reset per-run counters
	atomic.StoreInt64(&lastRunEvaluated, 0)
	atomic.StoreInt64(&lastRunFlagged, 0)
	atomic.StoreInt64(&lastRunFreedBytes, 0)

	// Evaluate
	evaluated := engine.EvaluateMedia(diskItems, prefs, rules)
	atomic.AddInt64(&metricsEvaluated, int64(len(evaluated)))
	atomic.StoreInt64(&lastRunEvaluated, int64(len(evaluated)))
	atomic.StoreInt64(&metricsLastRunEpoch, time.Now().Unix())

	// Sort by score descending
	sort.Slice(evaluated, func(i, j int) bool {
		return evaluated[i].Score > evaluated[j].Score // highest score first
	})

	targetBytesToFree := int64((currentPct - group.TargetPct) / 100.0 * float64(group.TotalBytes))
	if targetBytesToFree <= 0 {
		return
	}

	var bytesFreed int64

	// Track actioned shows to skip their child seasons (deduplication)
	actionedShows := make(map[string]bool)

	for _, ev := range evaluated {
		if bytesFreed >= targetBytesToFree {
			break
		}
		if ev.IsProtected || ev.Score <= 0 {
			continue
		}

		// Dedup: if this is a season and we already actioned the parent show, skip it
		if ev.Item.Type == integrations.MediaTypeSeason && ev.Item.ShowTitle != "" {
			if actionedShows[ev.Item.ShowTitle] {
				continue
			}
		}

		// Dedup: if this is a show, mark it so child seasons are skipped
		if ev.Item.Type == integrations.MediaTypeShow {
			actionedShows[ev.Item.Title] = true
		}

		actionName := "Dry-Run"
		if prefs.ExecutionMode == "auto" {
			client, ok := serviceClients[ev.Item.IntegrationID]
			if ok && client != nil {
				// Queue for background deletion so we don't block the poller
				select {
				case deleteQueue <- deleteJob{
					client:  client,
					item:    ev.Item,
					reason:  ev.Reason,
					score:   ev.Score,
					factors: ev.Factors,
				}:
					actionName = "Queued for Deletion"
					bytesFreed += ev.Item.SizeBytes
					continue // Skip the synchronous DB insert below, worker handles it
				default:
					slog.Warn("Deletion queue full, skipping item", "item", ev.Item.Title)
					continue
				}
			} else {
				slog.Error("Integration client not found for deletion", "itemID", ev.Item.IntegrationID)
				continue
			}
		} else if prefs.ExecutionMode == "approval" {
			actionName = "Queued for Approval"
		}

		factorsJSON, _ := json.Marshal(ev.Factors)
		logEntry := db.AuditLog{
			MediaName:    ev.Item.Title,
			MediaType:    string(ev.Item.Type),
			Reason:       fmt.Sprintf("Score: %.2f (%s)", ev.Score, ev.Reason),
			ScoreDetails: string(factorsJSON),
			Action:       actionName,
			SizeBytes:    ev.Item.SizeBytes,
			CreatedAt:    time.Now(),
		}
		db.DB.Create(&logEntry)

		bytesFreed += ev.Item.SizeBytes
		atomic.AddInt64(&metricsActioned, 1)
		atomic.AddInt64(&metricsFreedBytes, ev.Item.SizeBytes)
		atomic.AddInt64(&lastRunFlagged, 1)
		atomic.AddInt64(&lastRunFreedBytes, ev.Item.SizeBytes)
		slog.Info("Engine action taken", "media", ev.Item.Title, "action", actionName, "score", ev.Score, "freed", ev.Item.SizeBytes)
	}
}
