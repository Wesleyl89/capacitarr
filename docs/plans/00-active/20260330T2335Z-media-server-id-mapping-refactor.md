# Refactor Media Server ID Mapping (B+C Architecture)

**Status:** ✅ Complete — implemented in `feature/media-server-id-mapping`  
**Priority:** Architecture / Reliability  
**Estimated Effort:** L (2–3 days)

## Summary

Replace the current ephemeral full-library-scan TMDb→NativeID mapping with a persistent database table (approach B) backed by a targeted per-item search fallback (approach C). Add three-layer cleanup to prevent stale/orphaned mappings. This eliminates a class of silent failures affecting poster overlays, labels, and any future feature that needs to resolve TMDb IDs to media server item IDs.

## Problem Statement

The current architecture rebuilds the TMDb→NativeID map from scratch on every use by scanning all items in all media server libraries. This is fragile in multiple compounding ways:

| Problem | Impact |
|---------|--------|
| **Full library scan every cycle** | O(n) API calls where n = total library size, to resolve ~20 sunset items |
| **Ephemeral map, no persistence** | If Plex is unreachable during a cron cycle, all poster/label operations silently fail |
| **Silent skip on miss** | `UpdateOverlay` returns nil when TMDb ID can't be resolved; `UpdateAll` counts it as "updated" |
| **Missing query parameter broke everything** | `includeGuids=1` omission made the entire Plex map empty — zero guardrails detected this |
| **No incremental updates** | Adding one item to the sunset queue triggers a full library rescan on the next cycle |
| **Map built from wrong source** | `BuildTMDbToNativeIDMaps` iterates `labelManagers`, not `posterManagers` — works by coincidence |

### Root Cause

The mapping is treated as a transient computation rather than persistent data. Every consumer (posters, labels, future features) must independently build the map, handle failures, and thread it through the call stack via deps structs.

## Proposed Architecture: B+C

### Approach B: Persistent Mapping Table

A dedicated table storing resolved TMDb→NativeID mappings:

```sql
CREATE TABLE media_server_mappings (
    tmdb_id          INTEGER NOT NULL,
    integration_id   INTEGER NOT NULL,
    native_id        TEXT NOT NULL,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_id, integration_id),
    FOREIGN KEY (integration_id) REFERENCES integration_configs(id) ON DELETE CASCADE
);
```

- Populated during the engine poll cycle (which already fetches all items from media servers for enrichment)
- Used by poster overlay, label, and any future TMDb→NativeID consumer via simple DB lookup
- Survives media server downtime (stale data is better than no data)

### Approach C: Targeted Search Fallback

When the mapping table has a miss (item not found or stale), fall back to a per-item search against the media server:

```
Plex:     GET /hubs/search?query={title}&includeGuids=1
Jellyfin: GET /Items?searchTerm={title}&fields=ProviderIds
Emby:     GET /Items?SearchTerm={title}&Fields=ProviderIds
```

**Disambiguation:** The search returns candidates; we match on TMDb ID in the response metadata, not title alone. Title narrows the search space; TMDb ID is the authoritative match.

### Combined Flow

```
Consumer needs TMDb 871799 → Plex ratingKey
  │
  ├─ 1. Query media_server_mappings WHERE tmdb_id=871799 AND integration_id=1
  │     ├─ Found + fresh (updated_at < 7 days) → return native_id ✓
  │     ├─ Found + stale → return native_id, flag for background refresh
  │     └─ Not found → fall through to step 2
  │
  ├─ 2. Targeted search: GET /hubs/search?query=Pursuit&includeGuids=1
  │     ├─ Match found (tmdb://871799 in Guid array) → store mapping, return ✓
  │     └─ No match → log ERROR, return not-found ✗
  │
  └─ 3. On upload 404 (native_id stale):
        ├─ Delete stale mapping
        ├─ Re-run targeted search (step 2)
        └─ Retry upload with new native_id
```

## Three-Layer Cleanup

### Layer 1: Passive Verification During Use (Real-Time)

When a poster upload or label operation gets a 404 from the media server:

1. Delete the stale mapping for that (tmdb_id, integration_id)
2. Fall through to targeted search (C)
3. If search finds a new native_id → store it, retry the operation
4. If search finds nothing → item was removed from the media server; log error, optionally flag the sunset queue item

**Cost:** Zero — piggybacked on the 404 that already happened.

### Layer 2: Poll Cycle Freshness Refresh (Every 5 Minutes)

The engine poll already fetches all items from media servers for enrichment. During that fetch:

1. For each media server item with a TMDb ID:
   - Mapping exists + native_id matches → touch `updated_at`
   - Mapping exists + native_id differs → update `native_id` and `updated_at`
   - No mapping → insert
2. After processing all items from a media server:
   - Mappings for that integration whose `updated_at` was NOT touched this cycle → item no longer exists
   - Mark as unverified (don't delete yet — partial scan protection)

**Cost:** Zero extra API calls — piggybacked on the library scan already happening for scoring.

### Layer 3: Daily Garbage Collection (Cron)

A lightweight daily SQL cleanup:

```sql
-- Delete mappings unverified for 7+ days (item likely removed from media server)
DELETE FROM media_server_mappings
WHERE updated_at < datetime('now', '-7 days')
AND integration_id IN (SELECT id FROM integration_configs WHERE enabled = 1);

-- Delete orphaned mappings for removed integrations
DELETE FROM media_server_mappings
WHERE integration_id NOT IN (SELECT id FROM integration_configs);
```

**Cost:** One lightweight query per day.

### How the Layers Interact

| Scenario | Layer 1 | Layer 2 | Layer 3 |
|----------|---------|---------|---------|
| Plex ratingKey changed | 404 → delete + re-search | Detects mismatch, updates | N/A |
| Item deleted from Plex | 404 → delete + search fails | Marks unverified | Purges after 7 days |
| New item added to Plex | N/A | Inserts new mapping | N/A |
| Integration removed | N/A | N/A | Purges all orphaned mappings |
| Plex down for a weekend | Stale data still works | Skips refresh, no damage | 7-day window survives |
| Cold start (empty table) | Falls through to search (C) | First poll populates | N/A |

## What Gets Eliminated

The following current code becomes unnecessary:

- `BuildTMDbToNativeIDMaps()` — the full-library-scan map builder
- `GetTMDbToRatingKeyMap()` / `GetTMDbToItemIDMap()` — per-server map builders
- `TMDbToNativeID map[uint]map[int]string` — threaded through every deps struct
- Ephemeral map construction in every cron job and route handler
- The `cacheKeyForItem(integrationID, ...)` pattern — replaced by canonical TMDb-keyed lookups

## New Components

### 1. `MappingService` (new service)

```go
type MappingService struct {
    db  *gorm.DB
    bus *events.EventBus
}

// Resolve returns the native ID for a TMDb ID on a specific media server.
// Falls back to targeted search if not in the table.
func (s *MappingService) Resolve(tmdbID int, integrationID uint) (string, error)

// BulkUpsert updates/inserts mappings from a poll cycle batch.
func (s *MappingService) BulkUpsert(integrationID uint, mappings map[int]string) error

// InvalidateAndResolve deletes a stale mapping and attempts targeted search.
func (s *MappingService) InvalidateAndResolve(tmdbID int, integrationID uint, title string) (string, error)

// GarbageCollect removes stale and orphaned mappings.
func (s *MappingService) GarbageCollect(maxAge time.Duration) (int, error)
```

### 2. Targeted Search Methods (on media server clients)

```go
// SearchByTMDbID searches for an item by title and verifies the TMDb ID match.
func (p *PlexClient) SearchByTMDbID(title string, tmdbID int) (string, error)
func (j *JellyfinClient) SearchByTMDbID(title string, tmdbID int) (string, error)
func (e *EmbyClient) SearchByTMDbID(title string, tmdbID int) (string, error)
```

### 3. Database Migration

```sql
-- +goose Up
CREATE TABLE media_server_mappings (
    tmdb_id          INTEGER NOT NULL,
    integration_id   INTEGER NOT NULL,
    native_id        TEXT NOT NULL,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_id, integration_id),
    FOREIGN KEY (integration_id) REFERENCES integration_configs(id) ON DELETE CASCADE
);
CREATE INDEX idx_msm_integration ON media_server_mappings(integration_id);
CREATE INDEX idx_msm_updated ON media_server_mappings(updated_at);

-- +goose Down
DROP TABLE media_server_mappings;
```

## Implementation Phases

### Phase 1: Table + Bulk Population

1. Create migration for `media_server_mappings` table
2. Create `MappingService` with `BulkUpsert` and `Resolve` (table-only, no search fallback yet)
3. Integrate `BulkUpsert` into the engine poll cycle
4. Replace `BuildTMDbToNativeIDMaps()` callers with `MappingService.Resolve()`
5. Remove `TMDbToNativeID` from deps structs

### Phase 2: Targeted Search Fallback

1. Add `SearchByTMDbID` to Plex, Jellyfin, Emby clients
2. Add search fallback to `MappingService.Resolve()`
3. Add `InvalidateAndResolve` for 404 recovery
4. Wire 404 handling into poster upload and label application

### Phase 3: Cleanup Layers

1. Add passive verification (404 → invalidate + re-resolve) to poster and label operations
2. Add poll-cycle `updated_at` touching to Layer 2
3. Add daily `GarbageCollect` to the cron scheduler
4. Add observability: log mapping table size, hit/miss ratio, GC stats

### Phase 4: Cleanup Legacy Code

1. Remove `BuildTMDbToNativeIDMaps()`
2. Remove `GetTMDbToRatingKeyMap()` / `GetTMDbToItemIDMap()`
3. Remove `TMDbToNativeID` from `PosterDeps`, `SunsetDeps`, and cron builders
4. Remove `cacheKeyForItem(integrationID, ...)` pattern
5. Update tests

## Key Files (Current)

- `backend/internal/integrations/registry.go:302-347` — `BuildTMDbToNativeIDMaps()`
- `backend/internal/integrations/plex.go:347-364` — `GetTMDbToRatingKeyMap()`
- `backend/internal/integrations/jellyfin.go` — `GetTMDbToItemIDMap()`
- `backend/internal/integrations/emby.go` — `GetTMDbToItemIDMap()`
- `backend/internal/services/poster_overlay.go` — TMDb→NativeID consumers
- `backend/internal/services/sunset.go:569-599` — `applyLabel`/`removeLabel` TMDb→NativeID consumers
- `backend/internal/jobs/cron.go` — Ephemeral map construction

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Migration on large databases takes time | Table is empty on creation; no data migration needed |
| Targeted search returns wrong item | Match on TMDb ID in response metadata, not title |
| Race condition: poll updates mapping while poster uses old value | Stale native_id → 404 → Layer 1 re-resolves |
| Media server search API rate limits | Search is fallback only; primary path is table lookup |
| Table grows unbounded for large libraries | Layer 3 GC purges unverified mappings after 7 days |
