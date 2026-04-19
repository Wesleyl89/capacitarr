package poller

import (
	"fmt"
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
	anyDiskSuccess    bool // true if at least one disk reporter returned data without error
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
// healthy MediaSources, fetches disk space from healthy DiskReporters, and
// constructs the enrichment pipeline from discovered capabilities.
//
// The poller does NOT perform connection testing — that responsibility belongs
// to IntegrationHealthService. Instead, the poller consults the health service
// to determine which integrations are healthy and only fetches from those.
// Data fetch failures/successes are reported back to the health service via
// ReportFailure/ReportSuccess so health state stays current between ticks.
//
// Media fetches and disk fetches are parallelized within each section using
// goroutines. Results are merged sequentially after all goroutines complete
// to preserve deterministic logging and avoid concurrent map writes.
func fetchAllIntegrations(integrationSvc *services.IntegrationService, healthSvc *services.IntegrationHealthService) fetchResult {
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

	// Get healthy integration IDs from the health service
	healthyIDs := healthSvc.HealthyIDs()

	// ── Parallel media fetches ──────────────────────────────────────────
	// Fetch media items from healthy MediaSources concurrently. Unhealthy
	// integrations are skipped. Fetch failures are reported to the health
	// service; successes confirm the integration is reachable.
	mediaSources := registry.MediaSources()
	mediaResults := make([]mediaFetchResult, 0, len(mediaSources))
	var mediaMu sync.Mutex
	var mediaWg sync.WaitGroup

	for id, source := range mediaSources {
		if !healthyIDs[id] {
			slog.Debug("Skipping unhealthy integration for media fetch",
				"component", "poller", "integrationID", id)
			continue
		}
		mediaWg.Add(1)
		go func(id uint, source integrations.MediaSource) {
			defer mediaWg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("Panic recovered in media fetch goroutine",
						"component", "poller", "integrationID", id, "panic", r)
					mediaMu.Lock()
					mediaResults = append(mediaResults, mediaFetchResult{
						id:  id,
						err: fmt.Errorf("panic in media fetch: %v", r),
					})
					mediaMu.Unlock()
				}
			}()
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
			healthSvc.ReportFailure(mr.id, mr.err)
			continue
		}
		healthSvc.ReportSuccess(mr.id)
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
	// Fetch root folders and disk space from healthy DiskReporters concurrently.
	diskReporters := registry.DiskReporters()
	diskResults := make([]diskFetchResult, 0, len(diskReporters))
	var diskMu sync.Mutex
	var diskWg sync.WaitGroup

	for id, reporter := range diskReporters {
		if !healthyIDs[id] {
			slog.Debug("Skipping unhealthy integration for disk fetch",
				"component", "poller", "integrationID", id)
			continue
		}
		diskWg.Add(1)
		go func(id uint, reporter integrations.DiskReporter) {
			defer diskWg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("Panic recovered in disk fetch goroutine",
						"component", "poller", "integrationID", id, "panic", r)
					diskMu.Lock()
					diskResults = append(diskResults, diskFetchResult{
						id:        id,
						folderErr: fmt.Errorf("panic in disk fetch: %v", r),
						diskErr:   fmt.Errorf("panic in disk fetch: %v", r),
					})
					diskMu.Unlock()
				}
			}()
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
			healthSvc.ReportFailure(dr.id, dr.folderErr)
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
			healthSvc.ReportFailure(dr.id, dr.diskErr)
			continue
		}

		// Both folder and disk fetches succeeded for this integration
		if dr.folderErr == nil {
			healthSvc.ReportSuccess(dr.id)
		}

		result.anyDiskSuccess = true
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
