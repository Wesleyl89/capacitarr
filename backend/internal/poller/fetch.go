package poller

import (
	"log/slog"
	"sync"
	"time"

	"capacitarr/internal/integrations"
	"capacitarr/internal/services"
)

// fetchResult holds the aggregated results from fetching all integration data.
type fetchResult struct {
	allItems          []integrations.MediaItem
	rootFolders       map[string]bool
	diskMap           map[string]integrations.DiskSpace
	mountIntegrations map[string][]uint // mount path → integration IDs that reported it
	registry          *integrations.IntegrationRegistry
	pipeline          *integrations.EnrichmentPipeline
	brokenTypes       []string // integration types that failed connection testing
}

// connTestResult holds the outcome of a single connection test goroutine.
type connTestResult struct {
	id       uint
	err      error
	intType  string    // populated only on failure (for brokenTypes)
	testTime time.Time // when the test completed successfully
}

// mediaFetchResult holds the outcome of a single media source fetch goroutine.
type mediaFetchResult struct {
	id        uint
	items     []integrations.MediaItem
	err       error
	fetchTime time.Duration
}

// diskFetchResult holds the outcome of a single disk reporter fetch goroutine.
type diskFetchResult struct {
	id        uint
	folders   []string
	disks     []integrations.DiskSpace
	folderErr error
	diskErr   error
}

// fetchAllIntegrations builds an IntegrationRegistry, fetches media items from
// all MediaSources, fetches disk space from all DiskReporters, and constructs
// the enrichment pipeline from discovered capabilities.
//
// Connection tests, media fetches, and disk fetches are parallelized within each
// section using goroutines. This reduces wall-clock cycle time when multiple
// integrations are configured (e.g., Sonarr + Radarr fetch simultaneously).
// Results are merged sequentially after all goroutines complete to preserve
// deterministic logging and avoid concurrent map writes.
func fetchAllIntegrations(integrationSvc *services.IntegrationService) fetchResult {
	result := fetchResult{
		rootFolders:       make(map[string]bool),
		diskMap:           make(map[string]integrations.DiskSpace),
		mountIntegrations: make(map[string][]uint),
	}

	// Build the capability-based registry using factory pattern
	registry, err := integrationSvc.BuildIntegrationRegistry()
	if err != nil {
		slog.Error("Failed to build integration registry", "component", "poller", "error", err)
		return result
	}
	result.registry = registry

	// ── Parallel connection tests ───────────────────────────────────────
	// Test all connectable integrations concurrently. Each goroutine tests
	// one integration and records the result. DB updates happen sequentially
	// after all tests complete.
	connectors := registry.Connectors()
	connResults := make([]connTestResult, 0, len(connectors))
	var connMu sync.Mutex
	var connWg sync.WaitGroup

	for id, conn := range connectors {
		connWg.Add(1)
		go func(id uint, conn integrations.Connectable) {
			defer connWg.Done()
			cr := connTestResult{id: id, testTime: time.Now()}
			if connErr := conn.TestConnection(); connErr != nil {
				cr.err = connErr
				if cfg, cfgErr := integrationSvc.GetByID(id); cfgErr == nil {
					cr.intType = cfg.Type
				}
			}
			connMu.Lock()
			connResults = append(connResults, cr)
			connMu.Unlock()
		}(id, conn)
	}
	connWg.Wait()

	// Process connection results sequentially for deterministic logging + DB updates
	brokenSet := make(map[string]bool)
	for _, cr := range connResults {
		if cr.err != nil {
			slog.Error("Integration connection failed", "component", "poller",
				"integrationID", cr.id, "error", cr.err)
			if syncErr := integrationSvc.UpdateSyncStatus(cr.id, nil, cr.err.Error()); syncErr != nil {
				slog.Warn("Failed to update sync status after connection failure",
					"component", "poller", "integrationID", cr.id, "error", syncErr)
			}
			if cr.intType != "" {
				brokenSet[cr.intType] = true
			}
			continue
		}
		integrationSvc.PublishRecoveryIfNeeded(cr.id)
		if syncErr := integrationSvc.UpdateSyncStatus(cr.id, &cr.testTime, ""); syncErr != nil {
			slog.Warn("Failed to update sync status after successful connection",
				"component", "poller", "integrationID", cr.id, "error", syncErr)
		}
	}
	for t := range brokenSet {
		result.brokenTypes = append(result.brokenTypes, t)
	}

	// ── Parallel media fetches ──────────────────────────────────────────
	// Fetch media items from all MediaSources concurrently. Each goroutine
	// fetches from one integration. Post-processing (normalization, stats
	// updates, ShowLevelOnly filtering) runs sequentially after all fetches.
	mediaSources := registry.MediaSources()
	mediaResults := make([]mediaFetchResult, 0, len(mediaSources))
	var mediaMu sync.Mutex
	var mediaWg sync.WaitGroup

	for id, source := range mediaSources {
		mediaWg.Add(1)
		go func(id uint, source integrations.MediaSource) {
			defer mediaWg.Done()
			fetchStart := time.Now()
			items, fetchErr := source.GetMediaItems()
			mr := mediaFetchResult{
				id:        id,
				items:     items,
				err:       fetchErr,
				fetchTime: time.Since(fetchStart),
			}
			mediaMu.Lock()
			mediaResults = append(mediaResults, mr)
			mediaMu.Unlock()
		}(id, source)
	}
	mediaWg.Wait()

	// Process media results sequentially
	for _, mr := range mediaResults {
		if mr.err != nil {
			slog.Error("Media items fetch failed", "component", "poller",
				"integrationID", mr.id, "error", mr.err)
			continue
		}
		items := mr.items
		for i := range items {
			items[i].IntegrationID = mr.id
			items[i].Path = normalizePath(items[i].Path)
			if len(items[i].Collections) > 0 {
				items[i].CollectionSources = make(map[string]uint, len(items[i].Collections))
				for _, col := range items[i].Collections {
					items[i].CollectionSources[col] = mr.id
				}
			}
		}

		// When ShowLevelOnly is effectively enabled for this integration,
		// drop season-level items so only show-level entries are scored and
		// queued. The effective check considers both the stored setting and
		// virtual overrides (e.g., linked sunset-mode disk groups).
		effective, effErr := integrationSvc.IsShowLevelOnlyEffective(mr.id)
		if effErr == nil && effective {
			originalCount := len(items)
			filtered := items[:0]
			for _, item := range items {
				if item.Type != integrations.MediaTypeSeason {
					filtered = append(filtered, item)
				}
			}
			items = filtered

			source := "stored"
			if cfg, cfgErr := integrationSvc.GetByID(mr.id); cfgErr == nil && !cfg.ShowLevelOnly {
				source = "sunset-override"
			}
			slog.Debug("ShowLevelOnly filter applied", "component", "poller",
				"integrationID", mr.id, "removedSeasons", originalCount-len(items), "source", source)
		}

		result.allItems = append(result.allItems, items...)

		// Update media stats via service
		var totalSize int64
		mediaCount := len(items)

		hasShows := false
		for _, item := range items {
			if item.Type == integrations.MediaTypeShow {
				hasShows = true
				break
			}
		}

		if hasShows {
			mediaCount = 0
			for _, item := range items {
				if item.Type == integrations.MediaTypeShow {
					mediaCount++
					totalSize += item.SizeBytes
				}
			}
		} else {
			for _, item := range items {
				totalSize += item.SizeBytes
			}
		}
		if statsErr := integrationSvc.UpdateMediaStats(mr.id, totalSize, mediaCount); statsErr != nil {
			slog.Warn("Failed to update media stats",
				"component", "poller", "integrationID", mr.id, "error", statsErr)
		}
		slog.Debug("Media items fetched", "component", "poller",
			"integrationID", mr.id, "itemCount", len(items),
			"duration", mr.fetchTime.String())
	}

	// ── Parallel disk fetches ───────────────────────────────────────────
	// Fetch root folders and disk space from all DiskReporters concurrently.
	diskReporters := registry.DiskReporters()
	diskResults := make([]diskFetchResult, 0, len(diskReporters))
	var diskMu sync.Mutex
	var diskWg sync.WaitGroup

	for id, reporter := range diskReporters {
		diskWg.Add(1)
		go func(id uint, reporter integrations.DiskReporter) {
			defer diskWg.Done()
			folders, folderErr := reporter.GetRootFolders()
			disks, diskErr := reporter.GetDiskSpace()
			dr := diskFetchResult{
				id:        id,
				folders:   folders,
				disks:     disks,
				folderErr: folderErr,
				diskErr:   diskErr,
			}
			diskMu.Lock()
			diskResults = append(diskResults, dr)
			diskMu.Unlock()
		}(id, reporter)
	}
	diskWg.Wait()

	// Process disk results sequentially
	for _, dr := range diskResults {
		if dr.folderErr != nil {
			slog.Error("Root folder fetch failed", "component", "poller",
				"integrationID", dr.id, "error", dr.folderErr)
		}
		for _, f := range dr.folders {
			normalized := normalizePath(f)
			result.rootFolders[normalized] = true
			slog.Debug("Root folder found", "component", "poller",
				"integrationID", dr.id, "path", normalized)
		}

		if dr.diskErr != nil {
			slog.Error("Disk space fetch failed", "component", "poller",
				"integrationID", dr.id, "error", dr.diskErr)
			continue
		}

		for _, d := range dr.disks {
			if d.Path == "" {
				continue
			}
			d.Path = normalizePath(d.Path)
			slog.Debug("Disk space entry found", "component", "poller",
				"integrationID", dr.id, "path", d.Path,
				"totalBytes", d.TotalBytes, "freeBytes", d.FreeBytes)
			if existing, ok := result.diskMap[d.Path]; ok {
				if d.TotalBytes > existing.TotalBytes {
					result.diskMap[d.Path] = d
				}
			} else {
				result.diskMap[d.Path] = d
			}
			result.mountIntegrations[d.Path] = append(result.mountIntegrations[d.Path], dr.id)
		}
	}

	// Build the full enrichment pipeline via the shared function. This is
	// the single source of truth for pipeline construction — the cold-start
	// preview path (PreviewService.buildPreviewFromScratch) uses the same
	// function to avoid enrichment logic divergence.
	result.pipeline = integrations.BuildFullPipeline(registry)

	return result
}
