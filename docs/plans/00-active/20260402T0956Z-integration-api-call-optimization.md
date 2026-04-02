# Integration API Call Optimization

**Status:** ✅ Complete
**Created:** 2026-04-02
**Branch:** `refactor/integration-api-call-optimization`
**Category:** Performance / Backend

## Problem

A single poll cycle makes ~635 external API calls for a typical Sonarr + Radarr + Plex + Tautulli setup. Most are redundant:

- **Plex** `getMediaItems()` fetches the full library 5 times per cycle (collections, labels, watch data, TMDb mapping, mapping population)
- **Tautulli** makes ~600 per-item `get_history` calls instead of one bulk fetch
- **Jellyfin/Emby** call `getAllUsers()` ~6 times per cycle and fetch the Items listing ~3-4 times
- **Radarr** `ResolveCollectionMembers()` re-fetches the full movie list + quality profiles + tags

Target: reduce from ~635 to ~18 API calls per cycle (**97% reduction**).

## Design Principles

- **No stale data**: All optimizations are per-cycle caching only. Clients are created fresh each cycle by `BuildIntegrationRegistry()` and garbage-collected afterward. No data persists across cycles.
- **Consistent snapshots**: All enrichers within a cycle see the same library state (improvement over current behavior where enrichers see slightly different snapshots).
- **Same API endpoints**: No new API dependencies. Tautulli's `get_history` without `rating_key` is the same endpoint with a wider query.

## Phases

### Phase 1: Per-Cycle Library Cache (Plex, Jellyfin, Emby)

Since `BuildIntegrationRegistry()` creates new client instances each cycle, `sync.Once` caching is naturally cycle-scoped with zero lifecycle management.

#### Step 1.1: Plex Library Cache ✅
- [x] Add `cachedItems []MediaItem`, `cachedItemsErr error`, `cacheOnce sync.Once` fields to `PlexClient`
- [x] Rename `getMediaItems()` to `fetchMediaItems()` (the actual HTTP caller)
- [x] Create new `getMediaItems()` that uses `cacheOnce.Do()` to call `fetchMediaItems()` once
- [x] Refactor `GetBulkWatchData()` to use `getMediaItems()` instead of its own library fetch
- [x] All other callers (`GetCollectionMemberships`, `GetLabelMemberships`, `GetTMDbToRatingKeyMap`, `GetCollectionNames`, `GetLabelNames`) already use `getMediaItems()` — they benefit automatically
- [x] Existing Plex tests pass without modification

**Eliminates:** ~16 redundant Plex API calls per cycle

#### Step 1.2: Jellyfin Library/Users Cache ✅
- [x] Add `cachedUsers`, `usersOnce` for user list caching
- [x] Add `cachedAdminID`, `adminOnce` for admin user ID caching
- [x] Refactored `getAllUsers()` into cached wrapper + `fetchAllUsers()`
- [x] Refactored `GetAdminUserID()` into cached wrapper + `resolveAdminUserID()`
- [x] Existing Jellyfin tests pass without modification

**Eliminates:** ~5 redundant `/Users` calls per cycle

#### Step 1.3: Emby Library/Users Cache ✅
- [x] Same pattern as Jellyfin (mirrored codebase)
- [x] Existing Emby tests pass without modification

**Eliminates:** ~5 redundant `/Users` calls per cycle

### Phase 2: Bulk Tautulli History Fetch

#### Step 2.1: Add Bulk History Method ✅
- [x] Add `getAllHistory()` method to `TautulliClient` (unexported — only used by TautulliEnricher within same package)
- [x] Paginate `get_history` without `rating_key` filter, using `start` + `length=1000`
- [x] Returns `[]tautulliHistoryEntry` for in-memory aggregation
- [x] Page size constant `tautulliHistoryPageSize = 1000` for clarity

#### Step 2.2: Refactor TautulliEnricher ✅
- [x] Replaced per-item `GetWatchHistory()`/`GetShowWatchHistory()` loop with single `getAllHistory()` call
- [x] Aggregate history entries in-memory by `rating_key` (movies) and `grandparent_rating_key` (shows/episodes)
- [x] Build `map[string]*historyAgg` keyed by rating key with playCount, lastPlayed, users
- [x] Match against items using existing `tmdbToRatingKey` map
- [x] Used `string(MediaTypeEpisode)` constant to satisfy goconst linter
- [x] Existing Tautulli and enricher tests pass

**Eliminates:** ~598 individual API calls (from ~600 to ~2 paginated calls)

### Phase 3: Collection Resolver Data Reuse

#### Step 3.1: Cache Radarr Movie List ✅
- [x] Add `cachedMovies`, `cachedMoviesErr`, `moviesOnce` to `RadarrClient`
- [x] Add `getCachedMovies()` cached wrapper + `fetchMovies()` for raw HTTP call
- [x] Refactored `GetMediaItems()` to use `getCachedMovies()` instead of direct HTTP fetch
- [x] Refactored `ResolveCollectionMembers()` to use `getCachedMovies()` instead of re-fetching
- [x] Quality profile and tag lookups still done fresh in `ResolveCollectionMembers()` (different endpoint, low cost)
- [x] Existing Radarr tests pass

**Eliminates:** 3 API calls per collection resolution (movie list re-fetch)

### Phase 4: Parallel Integration Fetches

#### Step 4.1: Parallelize Independent Fetches ✅
- [x] Refactored `fetchAllIntegrations()` into three parallel sections using `sync.WaitGroup` + per-goroutine result structs
- [x] Parallel: connection tests (`connTestResult`), MediaSource fetches (`mediaFetchResult`), DiskReporter fetches (`diskFetchResult`)
- [x] Results collected via mutex-protected slices, merged sequentially after `wg.Wait()`
- [x] Post-processing (normalization, ShowLevelOnly filtering, stats updates) runs sequentially for deterministic behavior
- [x] Existing poller tests pass

**Impact:** Reduces wall-clock cycle duration when multiple integrations configured

### Phase 5: Verification ✅

- [x] `make ci` passes: lint (0 issues), all Go tests, govulncheck clean
- [x] Pre-existing `pnpm audit` vulnerabilities (lodash, cookie in `@vite-pwa/nuxt` transitive deps) are unrelated to this change
