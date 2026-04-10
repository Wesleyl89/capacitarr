# fix(plex): use session history API for multi-user play counts

**Status:** ✅ Complete
**Priority:** High (play counts silently wrong for all non-admin users)
**Date:** 2026-04-10

## Problem

Plex play counts only reflect the user whose `X-Plex-Token` is configured in
Capacitarr — typically the server admin. If admin watched a movie 0 times but
three other household users each watched it twice, Capacitarr reports
`PlayCount = 0` and the item is scored as unwatched.

This affects scoring (WatchHistoryFactor, RecencyFactor), custom rules
(`playcount`, `lastplayed`), and the `WatchedByUsers` field (never populated
by Plex).

Jellyfin and Emby are unaffected — they iterate all users in
`GetBulkWatchData()` and aggregate correctly.

Tautulli and Tracearr also aggregate correctly (priority 10), but users
shouldn't need a third-party tool for basic play counts from their own Plex
server.

## Root Cause

`PlexClient.GetBulkWatchData()` (plex.go:317-343) reads `viewCount` and
`lastViewedAt` from the cached library metadata. These fields come from
`GET /library/sections/{key}/all`, which returns **per-token-user** watch
state — a fundamental Plex API design choice.

```go
// Current: token-owner-scoped data
data := &WatchData{
    PlayCount:  item.PlayCount,   // ← from viewCount (admin-only)
    LastPlayed: item.LastPlayed,  // ← from lastViewedAt (admin-only)
}
// Users is never populated
```

## Fix

Replace the `viewCount`-based approach with Plex's session history endpoint:

```
GET /status/sessions/history/all
```

This endpoint returns individual play events for **all users** when called with
an admin token. Each entry includes `accountID`, `viewedAt`, `ratingKey`, and
`type`. The response is paginated via `X-Plex-Container-Start` and
`X-Plex-Container-Size` headers.

### Design: Large-Library-Friendly Bulk Fetch

The implementation mirrors how `TautulliClient.getAllHistory()` works — a
paginated bulk fetch with in-memory aggregation — but against the native Plex
API.

```
flowchart LR
    FETCH["Paginated fetch<br/>/status/sessions/history/all"] --> AGG["Aggregate by ratingKey<br/>sum plays, collect users,<br/>track most recent viewedAt"]
    AGG --> MAP["Build ratingKey→TMDbID<br/>reverse map from cached<br/>library metadata"]
    MAP --> RESULT["Return map[int]*WatchData<br/>keyed by TMDb ID"]
```

**Why this is large-library-friendly:**

1. **Single endpoint, paginated** — The history endpoint supports
   `X-Plex-Container-Size` (page size) and `X-Plex-Container-Start` (offset).
   A library with 50,000 play events at 1,000 per page = 50 HTTP calls, not
   50,000.
2. **No N+1 problem** — The alternative (`/library/metadata/{id}/users/top`)
   requires one API call per item. A 5,000-item library = 5,000 HTTP calls.
   The bulk history approach is O(total_plays / page_size) regardless of
   library size.
3. **Reuses existing cached data** — The ratingKey→TMDbID reverse map is built
   from `getMediaItems()` which is already cached per poll cycle. No additional
   library metadata fetches required.
4. **Bounded memory** — History entries are small (~200 bytes each). Even
   100,000 entries ≈ 20MB before aggregation, which collapses into a map of
   unique ratingKeys (bounded by library size, not play count).

### Response Format

```json
{
  "MediaContainer": {
    "size": 1000,
    "totalSize": 8432,
    "Metadata": [
      {
        "historyKey": "/status/sessions/history/12345",
        "key": "/library/metadata/218860",
        "ratingKey": "218860",
        "parentRatingKey": "218850",
        "grandparentRatingKey": "218800",
        "type": "episode",
        "viewedAt": 1712700000,
        "accountID": 1
      }
    ]
  }
}
```

### Aggregation Rules

| Media type | Group-by key | Rationale |
|------------|-------------|-----------|
| `movie` | `ratingKey` | Each movie is a standalone item |
| `episode` | `grandparentRatingKey` | Roll up all episode watches under the parent show, matching how *arr items are show-level |
| `track` | Skip | Music not supported |

Per aggregation group, track:
- **PlayCount** — number of history entries
- **LastPlayed** — most recent `viewedAt` across all entries
- **Users** — unique `accountID` values (resolved to usernames if possible,
  otherwise stored as string account IDs)

### Account ID → Username Resolution

The history endpoint returns numeric `accountID` values, not usernames. To
populate `WatchedByUsers` with human-readable names, we need a mapping.

**Option: `/accounts` endpoint** — `GET /accounts` returns all managed and
shared users with their `id` and `name` fields. This is a single lightweight
API call that can be cached for the poll cycle.

If the `/accounts` endpoint is unavailable or errors (e.g., non-admin token),
fall back to using `accountID` as a string (e.g., `"account:1"`). This
preserves the count semantics even without pretty names.

## Steps

- [x] Step 1: Create branch `fix/plex-multi-user-play-counts`

- [x] Step 2: Add Plex history response structs
  - Added `plexHistoryResponse` struct mapping the `/status/sessions/history/all`
    response (MediaContainer with `size`, `totalSize`, and Metadata array)
  - Added `plexHistoryEntry` struct with `ratingKey`, `parentRatingKey`,
    `grandparentRatingKey`, `type`, `viewedAt` (int64), `accountID` (int)
  - Added `plexAccountsResponse` and `plexAccount` structs for `/accounts`
  - Added `plexHistoryPageSize` constant (1000)

- [x] Step 3: Add `fetchAccounts()` method
  - Calls `GET /accounts` and returns `map[int]string` (accountID → username)
  - On error, returns empty map (non-fatal — caller falls back to numeric IDs)
  - Logs a debug message if the call fails (likely a non-admin token)

- [x] Step 4: Add `fetchAllHistory()` method
  - Paginates through `GET /status/sessions/history/all` using
    `X-Plex-Container-Start` and `X-Plex-Container-Size` query params
  - Uses a page size of 1000 (matching the Tautulli pattern)
  - Stops when `len(accumulated) >= totalSize` or an empty page is returned
  - Returns `[]plexHistoryEntry`
  - On mid-pagination error, returns accumulated entries with nil error
    (partial data is better than no data for large libraries)
  - On first-page error, returns nil entries with the error

- [x] Step 5: Add `fetchAllHistory()` pagination tests
  - `TestPlexClient_fetchAllHistory_SinglePage`
  - `TestPlexClient_fetchAllHistory_MultiPage` (verifies 2 API calls)
  - `TestPlexClient_fetchAllHistory_Empty`
  - `TestPlexClient_fetchAllHistory_MidPaginationError` (partial results returned)
  - `TestPlexClient_fetchAllHistory_FirstPageError` (error returned)
  - `TestPlexClient_fetchAllHistory_EntryDeserialization` (all fields verified)

- [x] Step 6: Rewrite `GetBulkWatchData()` to use history endpoint
  - Calls `fetchAccounts()` for accountID→username map
  - Calls `fetchAllHistory()` for all play events
  - Builds ratingKey→TMDbID reverse map from `getMediaItems()` (already cached)
  - Aggregates history entries by effective ratingKey:
    - Movies: `ratingKey`
    - Episodes: `grandparentRatingKey` (fall back to `ratingKey` if empty)
  - Per group: sums play count, tracks most recent `viewedAt`, collects unique
    account IDs → resolves to usernames via accounts map
  - Returns `map[int]*WatchData` keyed by TMDb ID with `PlayCount`,
    `LastPlayed`, and `Users` all populated
  - **Fallback:** If `fetchAllHistory()` returns an error with zero entries
    (total failure, not partial), falls back to the current `viewCount`-based
    approach via `getBulkWatchDataFallback()`. Logs a warning.
  - Added `buildEmptyWatchData()` for empty history (new server, no plays yet)
  - Zero-count entries included for library items with no history

- [x] Step 7: Update `GetBulkWatchData()` tests
  - Existing tests (Movies, Shows, DuplicateTMDbID, SkipsMissingTMDbGUID,
    EmptyLibrary, APIError) all pass via the fallback path since their mock
    servers don't serve the history endpoint
  - Added `TestPlexClient_GetBulkWatchData_MultiUserAggregation` — 3 users
    (mal, wash, zoe) watch Serenity with 2+1+3=6 total plays
  - Added `TestPlexClient_GetBulkWatchData_EpisodeAggregation` — episode
    history entries aggregate under grandparentRatingKey → show-level TMDb ID
  - Added `TestPlexClient_GetBulkWatchData_HistoryFallback` — history endpoint
    returns 500, verifies fallback to viewCount from library metadata
  - Added `TestPlexClient_GetBulkWatchData_AccountsFallback` — accounts
    endpoint fails, verifies "account:N" format used instead
  - Added `TestPlexClient_GetBulkWatchData_UnwatchedItemsIncluded` — verifies
    zero-count entries for unwatched library items
  - Added `newPlexMultiUserMockServer` test helper for multi-endpoint mocking

- [x] Step 8: `make ci` — lint and test stages passed
  - Note: `security:ci` (govulncheck) failed due to pre-existing Go 1.26.1
    stdlib vulnerabilities (crypto/tls, net/http, html/template) — unrelated
    to this change, requires Go 1.26.2 bump tracked separately

- [x] Step 9: Commit

## Files Changed

| File | Change |
|------|--------|
| `internal/integrations/plex.go` | Add history/accounts structs, `fetchAccounts()`, `fetchAllHistory()`, rewrite `GetBulkWatchData()` |
| `internal/integrations/plex_test.go` | Update existing tests, add multi-user and fallback tests |

## Not Changed

| File | Reason |
|------|--------|
| `enrichers.go` | `BulkWatchEnricher` is generic — it already handles `Users` if populated |
| `enrichment_pipeline.go` | Pipeline priorities unchanged — Tautulli/Tracearr still win at priority 10 |
| `types.go` | `WatchData` struct already has `Users []string` |
| Tautulli/Tracearr code | Not broken, not touched |
| Frontend | No UI changes — `WatchedByUsers` is already rendered when present |
