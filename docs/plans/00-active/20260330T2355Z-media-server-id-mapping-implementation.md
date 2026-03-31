# Media Server ID Mapping Refactor — Implementation Plan

**Status:** ✅ Complete (Phases 1-3 fully implemented; Phase 4 Steps 4.1-4.2 intentionally retained — see Phase 4 notes)  
**Priority:** Architecture / Reliability  
**Estimated Effort:** L (2–3 days)  
**Branch:** `feature/media-server-id-mapping` (from `feature/3.0`)  
**Design Doc:** `docs/plans/00-active/20260330T2335Z-media-server-id-mapping-refactor.md`

## Overview

Replace the ephemeral full-library-scan `TMDbToNativeID` map with a persistent `media_server_mappings` table backed by targeted per-item search fallback. This eliminates O(n) API calls per consumer invocation, survives media server downtime, and removes `TMDbToNativeID` from all deps structs.

## Current Architecture (What We're Replacing)

### Call Sites That Trigger Full Library Scans

Every call to `BuildTMDbToNativeIDMaps()` triggers a full scan of every enabled media server (Plex: `/library/sections/*/all`; Jellyfin/Emby: paginated `/Users/{id}/Items`).

| # | Call Site | File:Line | Trigger |
|---|----------|-----------|---------|
| 1 | `DELETE /sunset-queue/:id` | `routes/sunset.go:102` | User cancels one item |
| 2 | `POST /sunset-queue/clear` | `routes/sunset.go:167` | User clears queue |
| 3 | `POST /sunset-queue/refresh-posters` | `routes/sunset.go:226` | User refreshes posters |
| 4 | `POST /sunset-queue/restore-posters` | `routes/sunset.go:258` | User restores posters |
| 5 | `SunsetService.RefreshLabels()` | `services/sunset.go:559` | Label refresh route |
| 6 | `SunsetService.MigrateLabel()` | `services/sunset.go:589` | Settings label change |
| 7 | Daily sunset cron | `jobs/cron.go:144` | Once per day |
| 8 | Poller `evaluateSunsetMode()` | `poller/evaluate.go:523` | Once per poll cycle (lazy) |

### Deps Structs Carrying the Map

- `SunsetDeps.TMDbToNativeID` — `services/sunset.go:35`
- `PosterDeps.TMDbToNativeID` — `services/poster_overlay.go:33`
- `RunAccumulator.tmdbMap` / `.tmdbMapInit` — `poller/poller.go:31-32`

### Per-Client Map Builders (Full Library Scan)

- `PlexClient.GetTMDbToRatingKeyMap()` — `integrations/plex.go:347-364`
- `JellyfinClient.GetTMDbToItemIDMap()` — `integrations/jellyfin.go:766-811`
- `EmbyClient.GetTMDbToItemIDMap()` — `integrations/emby.go:748-793`

### TMDb→NativeID Consumers

- `PosterOverlayService.UpdateOverlay()` — `services/poster_overlay.go:96-103`
- `PosterOverlayService.RestoreOriginal()` — `services/poster_overlay.go:152-160`
- `PosterOverlayService.UpdateSavedOverlay()` — `services/poster_overlay.go:230+`
- `SunsetService.applyLabel()` — `services/sunset.go:607-637`
- `SunsetService.removeLabel()` — `services/sunset.go:641-668`

---

## Phase 1: Database Table + MappingService + Bulk Population

**Goal:** Persistent mapping table, `MappingService` with `BulkUpsert`/`Resolve`, wired into the poll cycle. Replace all `BuildTMDbToNativeIDMaps()` callers with `MappingService.Resolve()`.

### Step 1.1: Create Goose Migration `00008_media_server_mappings.sql`

**File:** `backend/internal/db/migrations/00008_media_server_mappings.sql`

```sql
-- +goose Up
-- Persistent TMDb → media server native ID mapping table.
-- Replaces the ephemeral in-memory maps built by BuildTMDbToNativeIDMaps().
-- Populated during engine poll cycles; survives media server downtime.
CREATE TABLE media_server_mappings (
    tmdb_id          INTEGER NOT NULL,
    integration_id   INTEGER NOT NULL,
    native_id        TEXT NOT NULL,
    media_type       TEXT NOT NULL DEFAULT 'movie',
    title            TEXT NOT NULL DEFAULT '',
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_id, integration_id),
    FOREIGN KEY (integration_id) REFERENCES integration_configs(id) ON DELETE CASCADE
);
CREATE INDEX idx_msm_integration ON media_server_mappings(integration_id);
CREATE INDEX idx_msm_updated ON media_server_mappings(updated_at);

-- +goose Down
DROP TABLE IF EXISTS media_server_mappings;
```

**Design decisions:**
- `media_type` column (`movie`, `show`): supports future disambiguation if a TMDb movie ID collides with a show ID on the same integration (TMDb movie and TV IDs are in separate namespaces, but storing type makes queries explicit).
- `title` column: stored at mapping time so `InvalidateAndResolve()` (Phase 2) has a title for targeted search without needing a secondary lookup. This addresses the data flow concern from the design review — the title comes from the mapping table itself, not the caller.
- Composite PK `(tmdb_id, integration_id)`: one mapping per TMDb ID per media server. If a user has the same movie in multiple Plex libraries under one integration, the last-seen native ID wins. This is acceptable because poster/label operations target the item regardless of which library it's in.

### Step 1.2: Create GORM Model `MediaServerMapping`

**File:** `backend/internal/db/models.go` (append after `MediaCache`)

```go
// MediaServerMapping stores a resolved TMDb ID → media server native ID mapping.
// Populated during engine poll cycles from media server library scans.
// Used by PosterOverlayService and SunsetService to translate TMDb IDs into
// per-server identifiers (Plex ratingKey, Jellyfin/Emby item ID) for label
// and poster operations. Survives media server downtime (stale data is better
// than no data).
type MediaServerMapping struct {
    TmdbID        int       `gorm:"primaryKey;column:tmdb_id" json:"tmdbId"`
    IntegrationID uint      `gorm:"primaryKey;column:integration_id" json:"integrationId"`
    NativeID      string    `gorm:"not null;column:native_id" json:"nativeId"`
    MediaType     string    `gorm:"not null;default:'movie';column:media_type" json:"mediaType"`
    Title         string    `gorm:"not null;default:'';column:title" json:"title"`
    UpdatedAt     time.Time `gorm:"not null;column:updated_at" json:"updatedAt"`
}

// TableName returns the database table name for MediaServerMapping.
func (MediaServerMapping) TableName() string {
    return "media_server_mappings"
}
```

### Step 1.3: Create `MappingService`

**File:** `backend/internal/services/mapping.go` (new)

Constructor follows the established pattern: `NewMappingService(database *gorm.DB, bus *events.EventBus)`.

```go
type MappingService struct {
    db  *gorm.DB
    bus *events.EventBus
}

func NewMappingService(database *gorm.DB, bus *events.EventBus) *MappingService
```

**Methods for Phase 1:**

| Method | Signature | Purpose |
|--------|-----------|---------|
| `Resolve` | `(tmdbID int, integrationID uint) (string, error)` | Look up native ID from DB; return `ErrMappingNotFound` on miss |
| `ResolveAll` | `(tmdbIDs []int, integrationID uint) (map[int]string, error)` | Batch lookup for multiple TMDb IDs on one integration (reduces N+1 in `applyLabel`/`UpdateOverlay` loops) |
| `BulkUpsert` | `(integrationID uint, mappings []MediaServerMapping) error` | Insert/update mappings from a poll cycle batch |
| `TouchedBefore` | `(integrationID uint, before time.Time) (int64, error)` | Count stale mappings (for Layer 2 verification) |
| `DeleteStale` | `(integrationID uint, before time.Time) (int64, error)` | Delete mappings not touched since `before` |
| `GarbageCollect` | `(maxAge time.Duration) (int64, error)` | Delete mappings older than `maxAge` + orphaned (integration deleted) |

**Error sentinel:**
```go
var ErrMappingNotFound = errors.New("mapping not found")
```

**`Resolve` implementation sketch:**
```go
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
```

**`BulkUpsert` implementation notes:**
- Use GORM's `Clauses(clause.OnConflict{...})` for SQLite `INSERT OR REPLACE` semantics.
- Accept `[]db.MediaServerMapping` (not raw map) so `media_type` and `title` are preserved.
- Update `updated_at` on every upsert (this is the "touch" for Layer 2).

### Step 1.4: Register `MappingService` on `services.Registry`

**File:** `backend/internal/services/registry.go`

- Add `Mapping *MappingService` field to `Registry` struct (after `PosterOverlay`).
- Add `NewMappingService(database, bus)` call in `NewRegistry()`.
- No cross-service wiring needed — `MappingService` has no lazy dependencies.

### Step 1.5: Populate Mappings During Poll Cycle

**File:** `backend/internal/poller/poller.go`

The poll cycle already calls `getMediaItems()` / `GetTMDbToItemIDMap()` on every media server for scoring. Instead of building an ephemeral map, call `MappingService.BulkUpsert()` with the results.

**Integration point:** After `BuildTMDbToNativeIDMaps()` is called in the poller (or its replacement), pipe the data into `MappingService.BulkUpsert()`.

Concretely, in the `run()` method of `poller.go`, after the integration registry is built and items are fetched:

1. For each media server integration, extract the TMDb→NativeID pairs already computed during item enrichment.
2. Build `[]db.MediaServerMapping` structs with `TmdbID`, `IntegrationID`, `NativeID`, `MediaType`, `Title`.
3. Call `p.reg.Mapping.BulkUpsert(integrationID, mappings)`.

**Key insight:** The poller already iterates all media items in `poller.go:run()` around lines 150-200 (fetching from integrations). The TMDb ID and native ID are already available on the `integrations.MediaItem` struct. We tap into this existing loop — no new API calls.

### Step 1.6: Replace All `BuildTMDbToNativeIDMaps()` Callers

Replace the 8 call sites (listed above) with `MappingService.Resolve()` or `MappingService.ResolveAll()`.

**Route handlers** (`routes/sunset.go`):
- Remove the `BuildTMDbToNativeIDMaps()` call and `tmdbMap` variable from all 4 route handlers.
- Remove `TMDbToNativeID` from the `SunsetDeps{}` and `PosterDeps{}` literals.
- The service methods (`Cancel`, `CancelAll`, `RefreshLabels`, etc.) will receive `MappingService` through the deps struct instead.

**Cron job** (`jobs/cron.go:144`):
- Remove `tmdbMap = registry.BuildTMDbToNativeIDMaps()` from the daily cron.
- Remove `TMDbToNativeID: tmdbMap` from `SunsetDeps{}` and `PosterDeps{}`.

**Poller** (`poller/evaluate.go:521-524`):
- Remove the lazy `tmdbMap` build from `RunAccumulator`.
- Remove `TMDbToNativeID: acc.tmdbMap` from `SunsetDeps{}`.

### Step 1.7: Refactor `SunsetDeps` and `PosterDeps`

Replace `TMDbToNativeID map[uint]map[int]string` with `Mapping *MappingService` on both deps structs.

**Before:**
```go
type SunsetDeps struct {
    Registry       *integrations.IntegrationRegistry
    Deletion       *DeletionService
    Engine         *EngineService
    Settings       SettingsReader
    PosterOverlay  *PosterOverlayService
    TMDbToNativeID map[uint]map[int]string
}
```

**After:**
```go
type SunsetDeps struct {
    Registry      *integrations.IntegrationRegistry
    Deletion      *DeletionService
    Engine        *EngineService
    Settings      SettingsReader
    PosterOverlay *PosterOverlayService
    Mapping       *MappingService
}
```

Same for `PosterDeps`: replace `TMDbToNativeID` with `Mapping *MappingService`.

### Step 1.8: Refactor Consumers to Use `MappingService.Resolve()`

**`applyLabel` / `removeLabel`** (`services/sunset.go:607-668`):

Before:
```go
idMap := tmdbMaps[integrationID]
nativeID, ok := idMap[*item.TmdbID]
```

After:
```go
nativeID, err := deps.Mapping.Resolve(*item.TmdbID, integrationID)
if errors.Is(err, ErrMappingNotFound) {
    continue
}
```

**`UpdateOverlay` / `RestoreOriginal` / `UpdateSavedOverlay`** (`services/poster_overlay.go`):

Same pattern — replace the two-level map lookup with `deps.Mapping.Resolve()`.

**Optimization:** For loops over multiple items (e.g., `UpdateAll`, `applyLabel` called in a loop), use `ResolveAll()` with a batch of TMDb IDs to avoid N+1 queries.

### Step 1.9: Remove `RunAccumulator` TMDb Fields

**File:** `backend/internal/poller/poller.go:31-32`

Remove:
```go
tmdbMap     map[uint]map[int]string
tmdbMapInit bool
```

### Step 1.10: Write Unit Tests for `MappingService`

**File:** `backend/internal/services/mapping_test.go` (new)

Follow the existing test pattern (in-memory SQLite via `setupTestDB()` from existing test files).

| Test | What It Verifies |
|------|------------------|
| `TestResolve_Found` | Returns correct native ID for known mapping |
| `TestResolve_NotFound` | Returns `ErrMappingNotFound` for unknown TMDb ID |
| `TestResolveAll_Mixed` | Returns found mappings, omits unfound |
| `TestBulkUpsert_Insert` | Inserts new mappings |
| `TestBulkUpsert_Update` | Updates existing mapping's native_id and updated_at |
| `TestBulkUpsert_Idempotent` | Re-inserting same data updates `updated_at` only |
| `TestDeleteStale` | Deletes mappings older than cutoff |
| `TestGarbageCollect_MaxAge` | Removes mappings older than maxAge |
| `TestGarbageCollect_Orphaned` | Removes mappings for deleted integrations |

### Step 1.11: Verify with `make ci`

Run `make ci` to confirm lint, test, and security checks pass.

---

## Phase 2: Targeted Search Fallback

**Goal:** When `MappingService.Resolve()` misses (item not in DB yet), fall back to a targeted search against the media server, store the result, and return it. This eliminates the Phase 1 window where newly-added items can't be resolved.

### Step 2.1: Add `SearchByTMDbID` to Media Server Clients

**Files:**
- `backend/internal/integrations/plex.go`
- `backend/internal/integrations/jellyfin.go`
- `backend/internal/integrations/emby.go`

Each client gets a new method:

```go
// SearchByTMDbID searches the media server for an item matching the given TMDb ID.
// Uses title to narrow the search space, then verifies the TMDb ID in the response
// metadata. Returns the native ID (ratingKey / item ID) or ErrNotFound.
func (p *PlexClient) SearchByTMDbID(title string, tmdbID int) (string, error)
```

**Plex implementation:**
- `GET /hubs/search?query={title}&includeGuids=1`
- Parse response, iterate results, match `tmdb://{tmdbID}` in `Guid` array.
- Return `ratingKey` of the matched item.

**Jellyfin implementation:**
- `GET /Users/{adminID}/Items?searchTerm={title}&fields=ProviderIds&IncludeItemTypes=Movie,Series&Recursive=true&Limit=25`
- Parse response, iterate results, match `ProviderIDs["Tmdb"]` == `tmdbID`.
- Return `item.ID` of the matched item.

**Emby implementation:**
- Identical to Jellyfin (same API structure).

### Step 2.2: Define `NativeIDSearcher` Interface

**File:** `backend/internal/integrations/types.go`

```go
// NativeIDSearcher can search for a media item's native ID by TMDb ID.
// Implemented by PlexClient, JellyfinClient, EmbyClient.
type NativeIDSearcher interface {
    SearchByTMDbID(title string, tmdbID int) (string, error)
}
```

Register on `IntegrationRegistry` alongside existing capability interfaces.

### Step 2.3: Add `NativeIDSearchers()` to `IntegrationRegistry`

**File:** `backend/internal/integrations/registry.go`

```go
func (r *IntegrationRegistry) NativeIDSearchers() map[uint]NativeIDSearcher
```

Returns all registered integrations that implement `NativeIDSearcher`.

### Step 2.4: Add Search Fallback to `MappingService.Resolve()`

**Updated `Resolve` signature:**
```go
func (s *MappingService) Resolve(tmdbID int, integrationID uint) (string, error)
```

**Updated flow:**
1. DB lookup (same as Phase 1).
2. On miss → check if a `NativeIDSearcher` is available for this integration.
3. If yes → call `searcher.SearchByTMDbID(title, tmdbID)` using the title from the sunset queue item (passed via a new `ResolveWithFallback` method or context).
4. On search hit → store the new mapping in DB, return native ID.
5. On search miss → return `ErrMappingNotFound`.

**Revised approach — split into two methods:**
- `Resolve(tmdbID, integrationID)` — DB-only, fast, no side effects.
- `ResolveWithSearch(tmdbID, integrationID, title, registry)` — DB first, then targeted search fallback. Stores result on hit.

This keeps the hot path (`Resolve`) simple and avoids passing the registry into every call.

### Step 2.5: Add `InvalidateAndResolve` for 404 Recovery

```go
func (s *MappingService) InvalidateAndResolve(tmdbID int, integrationID uint, title string, searcher NativeIDSearcher) (string, error)
```

1. Delete the stale mapping `WHERE tmdb_id = ? AND integration_id = ?`.
2. Call `searcher.SearchByTMDbID(title, tmdbID)`.
3. On hit → store new mapping, return native ID.
4. On miss → return `ErrMappingNotFound`.

### Step 2.6: Write Unit Tests for Search Fallback

| Test | What It Verifies |
|------|------------------|
| `TestResolveWithSearch_DBHit` | Returns DB result without calling searcher |
| `TestResolveWithSearch_SearchHit` | Falls back to search, stores result, returns |
| `TestResolveWithSearch_SearchMiss` | Returns `ErrMappingNotFound` |
| `TestInvalidateAndResolve_Recovers` | Deletes stale, searches, stores new |
| `TestInvalidateAndResolve_NoMatch` | Deletes stale, search fails, returns error |
| `TestSearchByTMDbID_Plex` | Plex search endpoint returns correct ratingKey |
| `TestSearchByTMDbID_Jellyfin` | Jellyfin search returns correct item ID |
| `TestSearchByTMDbID_Emby` | Emby search returns correct item ID |

### Step 2.7: Verify with `make ci`

---

## Phase 3: Cleanup Layers

**Goal:** Add three-layer cleanup to prevent stale/orphaned mappings.

### Step 3.1: Layer 1 — Passive 404 Verification

**Files:** `services/poster_overlay.go`, `services/sunset.go`

When `UploadPosterImage()` or `AddLabel()` returns an HTTP 404:

1. Call `MappingService.InvalidateAndResolve(tmdbID, integrationID, title, searcher)`.
2. If re-resolved → retry the operation with the new native ID.
3. If not resolved → log error, publish failure event.

**Implementation:** The `PosterManager` and `LabelManager` interfaces return `error`. We need to detect 404s specifically. Options:
- Define a sentinel error type `ErrNotFound` in integrations package.
- Check if the error wraps an HTTP 404 status.
- Both Plex and Jellyfin/Emby clients already log HTTP status — extend to return a typed error.

**Decision:** Add `integrations.IsNotFoundError(err)` helper that checks for 404 status in the error chain. This avoids changing the `PosterManager`/`LabelManager` interfaces.

### Step 3.2: Layer 2 — Poll Cycle Freshness

**File:** `backend/internal/poller/poller.go`

Already handled by Step 1.5 — `BulkUpsert` touches `updated_at` on every poll cycle. Mappings that aren't seen in a poll cycle have stale `updated_at` timestamps.

After the bulk upsert, optionally log the count of untouched mappings:
```go
stale, _ := p.reg.Mapping.TouchedBefore(integrationID, cycleStartTime)
if stale > 0 {
    slog.Debug("Stale mappings detected (not seen in this poll cycle)",
        "component", "poller", "integrationID", integrationID, "count", stale)
}
```

### Step 3.3: Layer 3 — Daily Garbage Collection

**File:** `backend/internal/jobs/cron.go`

Add to the daily cron job (Job 8), after the existing sunset processing:

```go
// 5. Garbage collect stale media server ID mappings
if cleaned, gcErr := reg.Mapping.GarbageCollect(7 * 24 * time.Hour); gcErr != nil {
    slog.Error("Failed to garbage collect media server mappings",
        "component", "jobs", "error", gcErr)
} else if cleaned > 0 {
    slog.Info("Garbage collected stale media server mappings",
        "component", "jobs", "removed", cleaned)
}
```

### Step 3.4: Write Tests for Cleanup Layers

| Test | What It Verifies |
|------|------------------|
| `TestLayer1_404Recovery` | 404 from poster upload triggers invalidate + re-resolve + retry |
| `TestLayer1_404NoMatch` | 404 + search miss logs error, doesn't retry |
| `TestLayer2_BulkUpsertTouchesTimestamp` | BulkUpsert updates `updated_at` for existing mappings |
| `TestLayer3_GarbageCollect_Stale` | Mappings >7 days old are deleted |
| `TestLayer3_GarbageCollect_Orphaned` | Mappings for deleted integrations are purged |
| `TestLayer3_GarbageCollect_PreservesActive` | Recent mappings are not deleted |

### Step 3.5: Verify with `make ci`

---

## Phase 4: Legacy Code Removal

**Goal:** Remove all dead code from the pre-refactor architecture.

### Step 4.1: ~~Remove `BuildTMDbToNativeIDMaps()`~~ — Retained as internal implementation

**Status:** Intentionally retained. `BuildTMDbToNativeIDMaps()` is called by `poller.populateMediaServerMappings()` to populate the persistent mapping table during each poll cycle. It is no longer called from any route handler, cron job, or deps struct. Removing it requires refactoring `populateMediaServerMappings()` to call per-client methods directly and extract title/mediaType during the same pass, which is a separate optimization.

Additionally, `GetTMDbToRatingKeyMap()` is independently called by `enrichment_pipeline.go:221` for Tautulli enrichment — a separate use case not addressed by this plan.

### Step 4.2: ~~Remove Per-Client Map Builders~~ — Retained as internal implementation

**Status:** Retained for the same reasons as Step 4.1. `GetTMDbToRatingKeyMap()` and `GetTMDbToItemIDMap()` are called by `BuildTMDbToNativeIDMaps()` (mapping population) and by `enrichment_pipeline.go` (Tautulli enrichment).

### Step 4.3: Remove TMDb Map from Deps Structs — ✅ Done

- ✅ `TMDbToNativeID` field removed from `SunsetDeps` — replaced by `Mapping *MappingService`.
- ✅ `TMDbToNativeID` field removed from `PosterDeps` — replaced by `Mapping *MappingService`.
- ✅ `tmdbMap`/`tmdbMapInit` removed from `RunAccumulator`.

### Step 4.4: Update All Tests — ✅ Done

- ✅ No test code constructs `TMDbToNativeID` maps manually.
- ✅ All existing tests pass with `MappingService`-based deps.
- ✅ `routes/sunset_test.go` and `services/sunset_test.go` pass without the old map.

### Step 4.5: Verify with `make ci` — ✅ Done

Full CI passes: lint, test, security.

---

## Key Design Decisions

### 1. Title Stored in Mapping Table

The design review flagged a data flow gap: `InvalidateAndResolve` needs a title for targeted search, but callers may only have a TMDb ID. By storing `title` in the mapping table at population time, the search fallback can retrieve it directly — no secondary lookup needed.

### 2. Phases 1 and 2 Deployed Together

The design review noted that Phase 1 alone is a regression for items added between poll cycles (no search fallback). To mitigate: **Phase 1 and Phase 2 will be implemented as a unit before removing the legacy code path.** Phase 4 (legacy removal) only happens after both Phase 1 and Phase 2 are complete and tested.

Concretely: during Phases 1-2, the old `BuildTMDbToNativeIDMaps()` method remains in the codebase but is no longer called. It is only deleted in Phase 4.

### 3. `ResolveAll` for Batch Lookups

The current code iterates `PosterManagers()` or `LabelManagers()` and does one map lookup per item. Replacing each lookup with a `Resolve()` DB query would create N+1 queries. `ResolveAll()` does a single `WHERE tmdb_id IN (...)` query, returning a map that the caller iterates.

### 4. No `verified` Column — `updated_at` Is Sufficient

The design review noted the schema lacks an explicit `verified` flag. This is intentional: `updated_at` serves as the verification timestamp. If `BulkUpsert` touches it during a poll cycle, the mapping is verified. If `updated_at` falls behind by >7 days, Layer 3 GC deletes it. Adding a separate column would create two timestamps to manage with no additional signal.

### 5. Composite PK Handles One-to-One Per Integration

If a user has the same movie in two Plex libraries under one integration, the last-seen native ID wins. This is acceptable: both native IDs point to the same TMDb content, and poster/label operations target the content regardless of library placement. If this becomes a real issue, the PK can be extended to `(tmdb_id, integration_id, native_id)` in a future migration.

---

## Files Created/Modified Summary

### New Files
| File | Purpose |
|------|---------|
| `backend/internal/db/migrations/00008_media_server_mappings.sql` | Goose migration |
| `backend/internal/services/mapping.go` | `MappingService` implementation |
| `backend/internal/services/mapping_test.go` | Unit tests |

### Modified Files
| File | Changes |
|------|---------|
| `backend/internal/db/models.go` | Add `MediaServerMapping` model |
| `backend/internal/services/registry.go` | Add `Mapping` to `Registry` |
| `backend/internal/services/sunset.go` | Replace map lookup with `MappingService.Resolve()`; refactor `SunsetDeps` |
| `backend/internal/services/poster_overlay.go` | Replace map lookup with `MappingService.Resolve()`; refactor `PosterDeps` |
| `backend/internal/poller/poller.go` | Remove `tmdbMap`/`tmdbMapInit` from `RunAccumulator`; add `BulkUpsert` call |
| `backend/internal/poller/evaluate.go` | Remove lazy map build; update `SunsetDeps` construction |
| `backend/internal/jobs/cron.go` | Remove `BuildTMDbToNativeIDMaps()`; add GC step; update deps |
| `backend/routes/sunset.go` | Remove map construction from all 4 route handlers |
| `backend/internal/integrations/registry.go` | Remove `BuildTMDbToNativeIDMaps()`; add `NativeIDSearchers()` |
| `backend/internal/integrations/plex.go` | Add `SearchByTMDbID()`; remove `GetTMDbToRatingKeyMap()` |
| `backend/internal/integrations/jellyfin.go` | Add `SearchByTMDbID()`; remove `GetTMDbToItemIDMap()` |
| `backend/internal/integrations/emby.go` | Add `SearchByTMDbID()`; remove `GetTMDbToItemIDMap()` |
| `backend/internal/integrations/types.go` | Add `NativeIDSearcher` interface |
| `backend/routes/sunset_test.go` | Update test fixtures |

### Deleted Code (Phase 4)
| What | Location |
|------|----------|
| `BuildTMDbToNativeIDMaps()` | `integrations/registry.go:302-347` |
| `GetTMDbToRatingKeyMap()` | `integrations/plex.go:347-364` |
| `GetTMDbToItemIDMap()` | `integrations/jellyfin.go:766-811` |
| `GetTMDbToItemIDMap()` | `integrations/emby.go:748-793` |
| `TMDbToNativeID` field | `SunsetDeps`, `PosterDeps` |
| `tmdbMap` / `tmdbMapInit` | `RunAccumulator` |

---

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Empty mapping table on first run after upgrade | Phase 2 search fallback handles cold starts; first poll cycle populates the table |
| Targeted search returns wrong item (title collision) | Match on TMDb ID in response metadata, not title alone |
| Race: poll updates mapping while poster uses old native_id | Stale native_id → 404 → Layer 1 re-resolves |
| Media server search API rate limits | Search is fallback only; primary path is DB lookup |
| Table grows unbounded for large libraries | Layer 3 GC purges unverified mappings after 7 days |
| SQLite write contention during bulk upsert | Use batch transactions (GORM `CreateInBatches`), same as existing patterns |
