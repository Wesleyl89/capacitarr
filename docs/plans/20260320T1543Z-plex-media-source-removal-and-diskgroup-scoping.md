# Remove Plex MediaSource & Fix DiskGroup Scoping

**Created:** 2026-03-20T15:43Z
**Status:** 🔲 Not Started
**Branch:** v2 (breaking changes allowed — no incremental migrations needed)

## Overview

Two related issues discovered during codebase audit:

1. **Plex (and potentially other media servers) registered as `MediaSource`** — adds unmanaged media items to the evaluation pool, inflating counts, polluting analytics, and causing deletion failures when items lack a `MediaDeleter` capability.

2. **Global operations that should be scoped to disk groups** — approval queue, preview cache, analytics, capacity forecast, dashboard stats, and audit log lack disk group context.

## Phase 1: Enforce MediaSource Exclusivity for Media Management Integrations

### Architectural Principle

**`MediaSource` must only be implemented by integrations that also implement `MediaDeleter` and `DiskReporter`** — i.e., integrations that authoritatively manage media content and can delete it (*arr integrations). Enrichment-only integrations (media servers, analytics services, request managers) must NEVER provide media items into the evaluation pool.

This is because:
- Only *arr integrations can delete what they report — Plex/Jellyfin/Emby/Tautulli/Seerr cannot
- Only *arr integrations report accurate disk space and root folders for DiskGroup matching
- Enrichment sources see ALL content including unmanaged items, causing duplicates and false positives
- Mixing authoritative and non-authoritative media items makes analytics unreliable

### Current Violations

| Integration | Currently implements MediaSource? | Should implement? | Action |
|---|---|---|---|
| Sonarr | ✅ | ✅ (authoritative) | No change |
| Radarr | ✅ | ✅ (authoritative) | No change |
| Lidarr | ✅ | ✅ (authoritative) | No change |
| Readarr | ✅ | ✅ (authoritative) | No change |
| **Plex** | **✅ (BUG)** | **❌** | **Remove** |
| Tautulli | ❌ | ❌ | Verified OK |
| Seerr | ❌ | ❌ | Verified OK |
| Jellyfin | ❌ | ❌ | Verified OK |
| Emby | ❌ | ❌ | Verified OK |

### Problem Details

`PlexClient` implements `MediaSource` (line 310 of `plex.go`), so the poller's `fetchAllIntegrations()` calls `GetMediaItems()` on Plex and merges those items into the same `allItems` pool alongside Sonarr/Radarr items. This causes:

- Duplicate items (Sonarr's "Firefly" + Plex's "Firefly" both appear)
- Unmanaged items (Plex-only content that no *arr manages) entering evaluation
- Deletion failures (Plex doesn't implement `MediaDeleter`)
- Inflated media stats per integration
- Polluted analytics (quality distribution, bloat, dead content include non-*arr items)

### Solution

Remove the `MediaSource` interface from `PlexClient`. Plex should only provide enrichment data via `WatchDataProvider` and `WatchlistProvider`. The capability comment in `types.go:37` should be updated to reflect:

```
Plex: Connectable + WatchDataProvider + WatchlistProvider
```

Note: `FetchCollectionValues()` in `integration.go:76-124` directly calls `client.GetMediaItems()` on Plex to extract collection names. This must be refactored to use a dedicated method or a lightweight collection-only API call rather than fetching all media items.

### Refactoring Detail: PlexClient.GetMediaItems()

`GetMediaItems()` is currently public and satisfies the `MediaSource` interface. However, it's also called internally by:

1. **`GetBulkWatchData()`** (`plex.go:245`) — calls `GetMediaItems()` to build a title→watch-data map for enrichment
2. **`FetchCollectionValues()`** (`integration.go:96-97`) — creates a temporary `PlexClient` and calls `GetMediaItems()` to extract collection names from all items

Neither of these callers needs the `MediaSource` interface — they just need the raw item data.

**Approach**: Rename `GetMediaItems()` → `getMediaItems()` (unexported). This:
- Removes `PlexClient` from the `MediaSource` interface (no exported `GetMediaItems`)
- Keeps `GetBulkWatchData()` working (calls the private method)
- Requires a new `GetCollectionNames() ([]string, error)` public method for collection extraction
- Requires `FetchCollectionValues()` to call the new `GetCollectionNames()` instead of `GetMediaItems()`

Note: The other enrichment sources (Jellyfin, Emby, Tautulli, Seerr) don't need this refactoring — they never had `GetMediaItems()` or `MediaSource`. They each use their own purpose-built methods for enrichment data.

### Steps

- [ ] **1.1** Rename `GetMediaItems()` → `getMediaItems()` (unexported) on `PlexClient` in `plex.go`
- [ ] **1.2** Update `GetBulkWatchData()` in `plex.go:245` to call `p.getMediaItems()` instead of `p.GetMediaItems()`
- [ ] **1.3** Remove `var _ MediaSource = (*PlexClient)(nil)` compile-time assertion from `plex.go:310`
- [ ] **1.4** Update capability comment in `types.go:36-41` to:
  - Remove `MediaSource` from Plex's line
  - Add an explicit note: "Only *arr integrations implement MediaSource — enrichment sources must NOT"
- [ ] **1.5** Add a new `GetCollectionNames() ([]string, error)` public method to `PlexClient`:
  - Calls `p.getMediaItems()` internally
  - Extracts and deduplicates collection tag strings from all items
  - Returns a sorted list of unique collection names
- [ ] **1.6** Refactor `FetchCollectionValues()` in `integration.go:76-124`:
  - Change `client.GetMediaItems()` → `client.GetCollectionNames()`
  - Remove the nested loop that extracted collections from items (now done by `GetCollectionNames()`)
  - Simplify the seen-map logic
- [ ] **1.7** Update all Plex tests in `plex_test.go`:
  - Remove `MediaSource` / `GetMediaItems` test cases
  - Add a negative compile-time assertion: `var _ MediaSource = (*PlexClient)(nil)` should NOT compile (verify via test comment)
  - Add tests for `GetCollectionNames()`
- [ ] **1.8** Verify all enrichment-only integrations do NOT implement `MediaSource`:
  - Jellyfin: `Connectable + WatchDataProvider + WatchlistProvider` ✅ (confirmed no `MediaSource`)
  - Emby: `Connectable + WatchDataProvider + WatchlistProvider` ✅ (confirmed no `MediaSource`)
  - Tautulli: `Connectable` ✅ (confirmed no `MediaSource`)
  - Seerr: `Connectable + RequestProvider` ✅ (confirmed no `MediaSource`)
- [ ] **1.9** Add a comment to the `IntegrationRegistry.Register()` method noting that if a client implements `MediaSource` but not `MediaDeleter`, it's likely a misconfiguration
- [ ] **1.10** Run `make ci` to verify no compilation errors or test failures

## Phase 2: Add DiskGroupID to Approval Queue and Audit Log

### Problem

`ApprovalQueueItem` and `AuditLogEntry` have no `DiskGroupID` field. Operations like `ClearQueue()`, `ListQueue()`, and audit log queries run globally with no ability to filter by which disk group triggered the action.

### Solution

Add `DiskGroupID` as a nullable FK to both models. The poller knows which disk group is being evaluated in `evaluateAndCleanDisk()` and can pass it through to `UpsertPending()` and `AuditLog.Create()`.

### Steps

- [ ] **2.1** Add `DiskGroupID *uint` field to `ApprovalQueueItem` model in `models.go` with JSON tag and gorm index
- [ ] **2.2** Add `DiskGroupID *uint` field to `AuditLogEntry` model in `models.go` with JSON tag and gorm index
- [ ] **2.3** Update baseline migration `00001_v2_baseline.sql`:
  - Add `disk_group_id INTEGER REFERENCES disk_groups(id) ON DELETE SET NULL` to `approval_queue` table
  - Add `disk_group_id INTEGER REFERENCES disk_groups(id) ON DELETE SET NULL` to `audit_log` table
  - Add indexes: `idx_approval_queue_disk_group_id`, `idx_audit_log_disk_group_id`
- [ ] **2.4** Manually update the running database to add the columns and indexes (SQL statements noted in plan for reference)
- [ ] **2.5** Verify that the migration detect logic (which compares running schema vs baseline) still works correctly

## Phase 3: Thread DiskGroupID Through the Poller

### Problem

The poller's `evaluateAndCleanDisk()` has the `group db.DiskGroup` parameter with the disk group ID, but doesn't pass it to `ApprovalService.UpsertPending()` or `AuditLogService.UpsertDryRun()`.

### Steps

- [ ] **3.1** Update `ApprovalService.UpsertPending()` to accept and store `DiskGroupID`
- [ ] **3.2** Update the `evaluate.go` approval-mode block (line 186-199) to pass `group.ID` as `DiskGroupID` in the `ApprovalQueueItem`
- [ ] **3.3** Update the `evaluate.go` dry-run block (line 216-229) to pass `group.ID` as `DiskGroupID` in the `AuditLogEntry`
- [ ] **3.4** Update the `DeleteJob` struct to include `DiskGroupID uint`
- [ ] **3.5** Update the auto-mode block in `evaluate.go` (line 158-171) to include `DiskGroupID` in the `DeleteJob`
- [ ] **3.6** Update `DeletionService.processJob()` to pass `DiskGroupID` through to `AuditLogService.Create()` when recording the deletion
- [ ] **3.7** Update `processForceDeletes()` in `evaluate.go` — force deletes may not have a disk group (they're threshold-independent), so `DiskGroupID` should be `nil` for these
- [ ] **3.8** Update relevant tests in `evaluate_test.go`, `approval_test.go`, `deletion_test.go`, and `auditlog_test.go`

## Phase 4: Scope Approval Queue Operations to DiskGroup

### Problem

- `ClearQueue()` deletes all pending/rejected items globally
- `ListQueue()` returns all items without disk group filtering
- `IsSnoozed()` checks globally

### Steps

- [ ] **4.1** Add `ClearQueueForDiskGroup(diskGroupID uint)` method to `ApprovalService` — clears only items belonging to a specific disk group
- [ ] **4.2** Update the poller's queue-clearing logic in `poller.go:244-251` — instead of a global `ClearQueue()` when all groups are below threshold, clear per-group as each below-threshold group is processed
- [ ] **4.3** Add `diskGroupID` filter parameter to `ListQueue()` (optional — nil means all)
- [ ] **4.4** Update the approval routes to accept an optional `disk_group_id` query parameter for filtering
- [ ] **4.5** Update `IsSnoozed()` to optionally scope by disk group (the poller has disk group context)
- [ ] **4.6** Update `BulkUnsnooze()` — consider per-disk-group semantics
- [ ] **4.7** Update approval tests

## Phase 5: Scope Analytics and Preview Cache

### Problem

The preview cache is a single global `*PreviewResult`. All analytics methods (`GetQualityDistribution`, `GetSizeAnomalies`, `GetStorageSunburst`, `GetDeadContent`, `GetStaleContent`, `GetLibraryStatusBreakdown`) operate on the full global item set with no disk group filtering.

### Solution

Rather than creating per-disk-group caches (which would multiply memory and complexity), add a filtering layer: analytics methods accept an optional `diskGroupID` and filter items by checking if the item's path falls under the disk group's mount path.

To enable this filtering, the preview cache items need to retain their `Path` field (they already do in `MediaItem`), and the analytics service needs access to disk group mount paths.

### Steps

- [ ] **5.1** Add a `DiskGroupID *uint` parameter to all analytics methods:
  - `GetQualityDistribution(diskGroupID *uint)`
  - `GetSizeAnomalies(diskGroupID *uint)`
  - `GetStorageSunburst(diskGroupID *uint)`
  - `GetDeadContent(minAgeDays int, diskGroupID *uint)`
  - `GetStaleContent(staleDays int, diskGroupID *uint)`
  - `GetLibraryStatusBreakdown(diskGroupID *uint)`
- [ ] **5.2** Add `DiskGroupLister` dependency to `AnalyticsService` and `WatchAnalyticsService` (they already use `PreviewDataSource`)
- [ ] **5.3** Implement a `filterItemsByDiskGroup()` helper method that:
  - Returns all items if `diskGroupID` is nil
  - Looks up the disk group's mount path and filters items by path prefix
- [ ] **5.4** Update all analytics route handlers to accept an optional `disk_group_id` query parameter and pass it through
- [ ] **5.5** Update `buildDiskContext()` in the preview service — currently picks the "best" group; should accept a `diskGroupID` parameter for targeted context
- [ ] **5.6** Update preview route handler to accept optional `disk_group_id`
- [ ] **5.7** Update analytics and preview tests

## Phase 6: Fix Capacity Forecast and Dashboard Stats

### Problem

- `analyticsForecastHandler()` hardcodes `groups[0]` — should accept a `disk_group_id` parameter
- `GetCapacityForecast()` fetches history without disk group filtering
- `GetDashboardStats()` library growth rate crosses disk groups randomly

### Steps

- [ ] **6.1** Update `analyticsForecastHandler()` to accept `disk_group_id` query parameter; default to the most degraded group (not just `groups[0]`)
- [ ] **6.2** Update `GetCapacityForecast()` to accept and pass through `diskGroupID` to `GetHistory()`
- [ ] **6.3** Update `GetDashboardStats()` to either:
  - Accept a `disk_group_id` parameter and scope all queries
  - Or aggregate correctly across all groups (sum totals, calculate weighted growth)
- [ ] **6.4** Update dashboard stats tests

## Phase 7: Run Full CI Verification

- [ ] **7.1** Run `make ci` from the `capacitarr/` directory
- [ ] **7.2** Fix any compilation errors, lint warnings, or test failures
- [ ] **7.3** Verify the migration detection logic works with the updated baseline

## Manual Database Update SQL

Since this is a breaking-change branch, the running database must be updated manually:

```sql
-- Add disk_group_id to approval_queue
ALTER TABLE approval_queue ADD COLUMN disk_group_id INTEGER REFERENCES disk_groups(id) ON DELETE SET NULL;
CREATE INDEX idx_approval_queue_disk_group_id ON approval_queue(disk_group_id);

-- Add disk_group_id to audit_log
ALTER TABLE audit_log ADD COLUMN disk_group_id INTEGER REFERENCES disk_groups(id) ON DELETE SET NULL;
CREATE INDEX idx_audit_log_disk_group_id ON audit_log(disk_group_id);
```

## Integration Capability Matrix (After Changes)

| Integration | Connectable | MediaSource | DiskReporter | MediaDeleter | WatchDataProvider | WatchlistProvider | RequestProvider | RuleValueFetcher |
|---|---|---|---|---|---|---|---|---|
| Sonarr | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Radarr | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Lidarr | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Readarr | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Plex | ✅ | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ |
| Tautulli | ✅ | ❌ | ❌ | ❌ | ❌* | ❌ | ❌ | ❌ |
| Seerr | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| Jellyfin | ✅ | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ |
| Emby | ✅ | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ |

*Tautulli uses per-item queries via `TautulliEnricher`, not the `WatchDataProvider` bulk interface.

## Files Modified

### Phase 1 (Plex MediaSource removal)
- `backend/internal/integrations/plex.go` — remove `GetMediaItems()`, remove `MediaSource` assertion
- `backend/internal/integrations/types.go` — update capability comment
- `backend/internal/integrations/plex_test.go` — remove MediaSource tests
- `backend/internal/services/integration.go` — refactor `FetchCollectionValues()`

### Phase 2 (Schema changes)
- `backend/internal/db/models.go` — add `DiskGroupID` to `ApprovalQueueItem` and `AuditLogEntry`
- `backend/internal/db/migrations/00001_v2_baseline.sql` — add columns and indexes

### Phase 3 (Poller threading)
- `backend/internal/poller/evaluate.go` — pass disk group ID through all code paths
- `backend/internal/services/approval.go` — accept disk group ID in `UpsertPending()`
- `backend/internal/services/deletion.go` — `DeleteJob` gains `DiskGroupID`
- `backend/internal/services/auditlog.go` — `Create()` and `UpsertDryRun()` gain disk group context

### Phase 4 (Approval queue scoping)
- `backend/internal/services/approval.go` — new per-group methods
- `backend/internal/poller/poller.go` — per-group queue clearing
- `backend/routes/approval.go` — disk group filter parameter

### Phase 5 (Analytics scoping)
- `backend/internal/services/analytics.go` — disk group filter parameter
- `backend/internal/services/watch_analytics.go` — disk group filter parameter
- `backend/internal/services/preview.go` — disk group aware build context
- `backend/routes/analytics.go` — query parameter

### Phase 6 (Forecast/dashboard scoping)
- `backend/internal/services/metrics.go` — disk group scoping
- `backend/routes/analytics.go` — forecast handler
- `backend/routes/metrics.go` — dashboard stats handler
