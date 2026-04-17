# Fix: Time-in-Library Data Source Accuracy

**Status:** ✅ Complete
**Branch:** `fix/time-in-library-accuracy`
**Created:** 2026-04-16

## Problem

The `time_in_library` scoring factor and rule condition use `AddedAt` on `MediaItem`, which is currently populated from the *arr `added` field. This represents when the entry was **created in the *arr database** (e.g., when a user clicked "Add Movie"), NOT when the media file was actually downloaded/imported to disk.

**Example:** A user adds "Serenity" to Radarr on 2026-01-01 but the movie doesn't download until 2026-04-01. The `time_in_library` factor reports ~3.5 months when the file has been on disk for ~2 weeks.

**Sonarr compounding issue:** All seasons of a show inherit the show-level `added` date. Season 6 downloaded yesterday gets the same age as Season 1 from 3 years ago.

## Solution: Layered Date Resolution

Use a fallback chain for `AddedAt`:
1. **Media server library date** (Plex `addedAt`, Jellyfin/Emby `DateCreated`) — most accurate, reflects when the file was scanned into the library
2. **\*arr file-level `dateAdded`** (Radarr `movieFile.dateAdded`, Sonarr `episodeFile.dateAdded`, etc.) — when the file was imported into the *arr
3. **\*arr entry-level `added`** — when the entry was created in the *arr database (current behavior, last resort)

## Affected Files

- `backend/internal/integrations/radarr.go` — `radarrMovie` struct, `GetMediaItems()`, `ResolveCollectionMembers()`
- `backend/internal/integrations/sonarr.go` — `sonarrSeries` struct, `GetMediaItems()`
- `backend/internal/integrations/lidarr.go` — `lidarrArtist` struct, `GetMediaItems()`
- `backend/internal/integrations/readarr.go` — `readarrBook` struct, `GetMediaItems()`
- `backend/internal/integrations/plex.go` — `GetBulkWatchData()` (expose `addedAt`)
- `backend/internal/integrations/jellyfin.go` — `jellyfinItem` struct (add `DateCreated`)
- `backend/internal/integrations/emby.go` — `embyItem` struct (add `DateCreated`)
- `backend/internal/integrations/enrichers.go` — new `LibraryDateEnricher`
- `backend/internal/integrations/enrichment_pipeline.go` — register new enricher
- `backend/internal/integrations/types.go` — add `AddedAt` to `WatchData`
- Tests for all of the above

## Phase 1: Fix *arr File-Level Dates

### Step 1.1: Radarr — Parse `movieFile.dateAdded`

The Radarr `/api/v3/movie` response already includes an inline `movieFile` object with `dateAdded`. We just need to parse it.

- [ ] Add `radarrMovieFile` struct with `DateAdded string`
- [ ] Add `MovieFile *radarrMovieFile` field to `radarrMovie` struct
- [ ] In `GetMediaItems()`: prefer `movieFile.dateAdded` over `movie.added`, fall back if nil/empty
- [ ] In `ResolveCollectionMembers()`: same logic
- [ ] Add helper function `radarrResolveAddedAt(movieFile *radarrMovieFile, added string) *time.Time`
- [ ] Update `radarr_test.go`: add `movieFile` to test fixtures, test both present and absent scenarios

### Step 1.2: Sonarr — Query Episode File Dates Per Season

Sonarr's `/api/v3/series` response does NOT include per-episode file dates. The `/api/v3/episodefile?seriesId=X` endpoint returns episode files with `dateAdded`.

- [ ] Add `sonarrEpisodeFile` struct with `ID`, `SeasonNumber`, `DateAdded`
- [ ] After fetching series, batch query `/api/v3/episodefile?seriesId=X` per series (only for series with `SizeOnDisk > 0`)
- [ ] Build a `map[int]map[int]time.Time` — `seriesID -> seasonNumber -> maxDateAdded`
- [ ] For season items: use `maxDateAdded` for that season, fall back to `series.added`
- [ ] For show-level items: use `maxDateAdded` across all seasons, fall back to `series.added`
- [ ] Update `sonarr_test.go`: mock episodefile endpoint, test date resolution

### Step 1.3: Lidarr — Artist-Level `added` (No File-Level Improvement)

Lidarr only emits artist-level items. The `/api/v1/trackfile` endpoint has `dateAdded`, but querying it per artist adds API calls for marginal benefit (artist = collection of albums, `added` date is reasonable).

- [ ] **Decision: Keep current behavior.** Lidarr `artist.added` is acceptable because artists are managed as a unit. Document this decision.

### Step 1.4: Readarr — Uses `book.added` (No Inline File Object)

**Verified:** Unlike Radarr (which embeds `movieFile` inline), Readarr does NOT include
`bookFile` in the `/api/v1/book` response. The `BookFile` type is only available via
the separate `/api/v1/bookfile` endpoint (confirmed via golift.io/starr SDK and Readarr
API docs). Since books are individual items (not seasons) and the gap between adding a
book and it downloading is typically small, we keep `book.added` as the `AddedAt` date.
The media server enrichment layer (Phase 2) still overrides this with a more accurate
date when Plex/Jellyfin/Emby is configured.

- [x] Checked Readarr API — `bookFile` is NOT inline (separate endpoint only)
- [x] **Decision: Keep `book.added`.** Documented in `readarr.go` comment.

## Phase 2: Media Server LibraryDateEnricher

### Step 2.1: Extend `WatchData` Struct

- [ ] Add `AddedAt *time.Time` field to `WatchData` in `types.go`

### Step 2.2: Populate `AddedAt` in Plex `GetBulkWatchData()`

Plex already parses `addedAt` into `MediaItem` objects internally. Bridge it to `WatchData`.

- [ ] In `GetBulkWatchData()`: when building `WatchData` entries, populate `AddedAt` from the Plex item's `addedAt` timestamp
- [ ] Update Plex tests to verify `AddedAt` is set on `WatchData`

### Step 2.3: Populate `AddedAt` in Jellyfin

- [ ] Add `DateCreated string` to `jellyfinItem` struct
- [ ] In `GetBulkWatchDataForUser()`: parse `DateCreated` and set `WatchData.AddedAt`
- [ ] For show-level items: use the earliest/latest `DateCreated` across episodes (whichever makes more sense — latest = most recent file added)
- [ ] Update Jellyfin tests

### Step 2.4: Populate `AddedAt` in Emby

- [ ] Add `DateCreated string` to `embyItem` struct
- [ ] In `GetBulkWatchDataForUser()`: parse `DateCreated` and set `WatchData.AddedAt`
- [ ] Same aggregation logic as Jellyfin
- [ ] Update Emby tests

### Step 2.5: Create `LibraryDateEnricher`

New enricher that overrides `AddedAt` on items using media server library dates from `WatchData`.

- [ ] Add new enrichment capability constant: `EnrichCapLibraryDate = "library_date"`
- [ ] Create `LibraryDateEnricher` struct in `enrichers.go`
- [ ] Implementation: iterate items, match by TMDb ID, override `AddedAt` from `WatchData.AddedAt`
- [ ] Priority: high (run after watch data enrichers so `WatchData.AddedAt` is populated)
- [ ] Register in `BuildEnrichmentPipeline()` for all `WatchDataProvider` integrations
- [ ] Update enricher tests

### Step 2.6: Update `BulkWatchEnricher` to Bridge `AddedAt`

Alternatively to a separate enricher, extend the existing `BulkWatchEnricher` to also set `AddedAt` when the `WatchData` has it and the item's *arr date is older.

- [ ] In `BulkWatchEnricher.Enrich()`: if `wd.AddedAt != nil` and (item has no `AddedAt` OR media server date is more recent), override `item.AddedAt`
- [ ] Same logic in `TautulliEnricher` (Tautulli doesn't have library dates, skip)
- [ ] Same logic in `JellystatEnricher` (Jellystat may not have library dates, check)

## Phase 3: Verification

- [ ] Run `make ci` — all lint, test, and security checks pass
- [ ] Manual verification with Docker: compare old vs new `AddedAt` values in evaluation results
