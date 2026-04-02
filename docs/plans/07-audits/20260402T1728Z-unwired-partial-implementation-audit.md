# Unwired & Partial Implementation Audit

**Created:** 2026-04-02T17:28Z
**Status:** Complete (29 of 29 findings remediated)
**Type:** Audit
**Scope:** Full codebase — backend, frontend, integrations, events, notifications, database, OpenAPI

---

## Summary

A comprehensive audit of the Capacitarr codebase to identify code that is:

1. **Defined but never called** (dead code)
2. **Partially implemented** (wired on one end but not the other)
3. **Claimed in plans as complete but actually missing or broken**
4. **Inconsistent across layers** (backend vs frontend vs database vs OpenAPI)

The audit examined every Go package, every Vue component/composable, every event type, every integration, the database schema, and the OpenAPI specification. All findings were individually verified with codebase searches.

---

## Findings Index

| # | Severity | Category | Title |
|---|----------|----------|-------|
| F-01 | Critical | Backend Dead Code | `PosterOverlayService.UpdateSavedOverlay()` never called |
| F-02 | Critical | Backend Dead Code | `SunsetService.MigrateLabel()` never called |
| F-03 | Critical | Notifications | `AlertSunsetActivity` defined but never dispatched |
| F-04 | High | Integrations Bug | Registry `Unregister()`/`Clear()` skip `nativeIDSearchers` |
| F-05 | High | Integrations Gap | TracearrEnricher does not set `LastPlayed` |
| F-06 | High | Integrations Gap | `DetectEnrichment()` omits Tracearr |
| F-07 | High | OpenAPI | Notification channel schemas are stale (pre-migration-00010) |
| F-08 | High | OpenAPI | `LibraryHistory` field name capitalization mismatch |
| F-09 | High | OpenAPI | Ghost `reason` field on `AuditLogEntry` and `ApprovalQueueItem` |
| F-10 | Medium | OpenAPI | 10 API endpoints missing from OpenAPI spec |
| F-11 | Medium | OpenAPI | `info.version` stale at `2.0.0` |
| F-12 | Medium | OpenAPI | `action` enum includes nonexistent `dry_run` |
| F-13 | Medium | Frontend SSE | `approval_returned_to_pending` — no frontend handler |
| F-14 | Medium | Frontend SSE | `data_reset` — no frontend handler |
| F-15 | Medium | Frontend SSE | `settings_imported` — no frontend handler |
| F-16 | Medium | Frontend SSE | `analytics_updated` — no frontend handler |
| F-17 | Medium | Frontend | `deadContentMinDays` / `staleContentDays` — no settings UI |
| F-18 | Medium | Backend | `EngineRunStats.ExecutionMode` — per-disk-group mode tracking TODO |
| F-19 | Low | Backend Dead Code | `IntegrationService.Update()` never called outside tests |
| F-20 | Low | Backend Dead Code | `NotificationChannelService.Update()` never called outside tests |
| F-21 | Low | Backend Dead Code | `SunsetService.CancelAllForDiskGroup()` never called outside tests |
| F-22 | Low | Backend Dead Code | `MappingService.DeleteStale()` never called outside tests |
| F-23 | Low | Frontend Dead Code | `rules.vue` stale `prefs` reactive object |
| F-24 | Low | Frontend Dead Code | Unused type definitions in `api.ts` |
| F-25 | Low | Frontend Dead Code | Unused function exports |
| F-26 | Low | Frontend | Orphaned locale keys (notification center UI) |
| F-27 | Low | Database | `album` media type in CHECK constraint but unused |
| F-28 | Low | Database | `EngineRunStats` has `created_at` column but no Go field |
| F-29 | Low | OpenAPI | Settings export/import use undefined tag `[Settings]` |

---

## Detailed Findings

### F-01: `PosterOverlayService.UpdateSavedOverlay()` never called

**Severity:** Critical
**Files:**
- `backend/internal/services/poster_overlay.go:215` (definition)

**Description:**
This method generates a green "Saved by popular demand" poster overlay for sunset items that have been rescued. The image composition code is fully implemented — it renders a shield-check icon with "SAVED" text. However, no code anywhere in the codebase calls this method.

The `RescoreAndSave()` method in `sunset.go` marks items as "saved" and publishes a `SunsetSavedEvent`, but it never updates the poster. The daily cron job's poster overlay update step (`UpdateAll()`) only processes `pending`-status items, not `saved` items.

**Impact:** Items saved by popular demand retain their countdown overlay ("Leaving in X days") instead of showing the intended green "Saved" badge. Users see misleading poster overlays.

**Plan claim:** The sunset implementation plan (`20260329T1404Z`) lists poster overlay for saved items as part of Phase 2 and marks it complete.

**Remediation:** Wire `UpdateSavedOverlay()` into the `RescoreAndSave()` flow in `sunset.go`, or into the daily cron's poster update step for items with `saved` status.

---

### F-02: `SunsetService.MigrateLabel()` never called

**Severity:** Critical
**Files:**
- `backend/internal/services/sunset.go:597` (definition)

**Description:**
This method handles the case where a user changes the `sunsetLabel` preference (e.g., from `capacitarr-sunset` to a custom label). It removes the old label and applies the new one across all sunset queue items in all enabled media servers. The method is fully implemented with proper error handling and event publishing.

However, nothing calls it. The settings save handler (`PATCH /preferences/sunset`) does not detect sunset label changes and does not invoke `MigrateLabel()`. The daily cron's `RefreshLabels()` only applies labels to unlabeled items — it does not migrate existing labels.

**Impact:** Changing the sunset label setting in preferences leaves orphaned labels on media server items. The old label persists on items already in the sunset queue, and the new label is only applied to newly-queued items.

**Plan claim:** The sunset implementation plan (`20260329T1404Z`) lists label migration as a requirement. The method exists but was never wired to the preference save path.

**Remediation:** Add sunset label change detection to the `PATCH /preferences/sunset` handler (or its service method) and call `MigrateLabel()` when the label value changes.

---

### F-03: `AlertSunsetActivity` defined but never dispatched

**Severity:** Critical
**Files:**
- `backend/internal/notifications/sender.go:138` (constant definition)
- `backend/internal/notifications/sender.go:169` (trigger label)
- `backend/internal/notifications/sender.go:386` (alert color)
- `backend/internal/notifications/apprise.go:120` (Apprise priority mapping)
- `backend/internal/services/notification_dispatch.go` (missing case)

**Description:**
`AlertSunsetActivity` is defined as a notification alert type with:
- A constant value (`"sunset_activity"`)
- A human-readable trigger label (`"Sunset Activity"`)
- A color mapping (`ColorAmber`)
- An Apprise priority mapping

The rendering pipeline is fully wired — if an alert of this type were created, it would be correctly formatted and sent. However, `NotificationDispatchService.handle()` never creates an `Alert{Type: AlertSunsetActivity}`. Sunset events (`SunsetEscalatedEvent`, `SunsetMisconfiguredEvent`, `SunsetExpiredEvent`, `SunsetSavedEvent`) are either routed to `AlertThresholdBreached` or `AlertError`, or accumulated into the digest enrichment.

Additionally, `NotificationConfig` has no `OverrideSunsetActivity` field, and `resolveOverride()` has no `"sunset_activity"` case. The notification tiers plan (`20260402T1211Z`) explicitly acknowledges this gap but does not resolve it — it integrates sunset events into existing categories rather than creating a dedicated sunset notification type.

**Impact:** Users cannot receive dedicated sunset activity notifications. Sunset events are folded into generic categories, losing specificity.

**Remediation:** Either:
1. Wire sunset events to dispatch `AlertSunsetActivity` alerts and add `OverrideSunsetActivity` to `NotificationConfig` + migration
2. Or remove the dead `AlertSunsetActivity` constant and related rendering code if the design decision is to keep sunset events in generic categories

---

### F-04: Registry `Unregister()`/`Clear()` skip `nativeIDSearchers`

**Severity:** High
**Files:**
- `backend/internal/integrations/registry.go:121-123` (Register adds to `nativeIDSearchers`)
- `backend/internal/integrations/registry.go:131-148` (Unregister misses it)
- `backend/internal/integrations/registry.go:151-168` (Clear misses it)

**Description:**
The `IntegrationRegistry` has 14 capability maps. `Register()` correctly populates all 14, including `nativeIDSearchers`. However, `Unregister()` deletes from only 13 maps and `Clear()` re-initializes only 13 maps — both skip `nativeIDSearchers`.

This was likely introduced when `NativeIDSearcher` was added (it is the newest capability, used by the media server ID mapping feature) and `Unregister()`/`Clear()` were not updated.

**Impact:** When an integration is removed (`Unregister()`) or the registry is cleared (`Clear()`), its `NativeIDSearcher` registration persists as a stale entry. The `MappingService` could attempt searches against a disconnected integration.

**Remediation:** Add `delete(r.nativeIDSearchers, id)` to `Unregister()` and `r.nativeIDSearchers = make(...)` to `Clear()`.

---

### F-05: TracearrEnricher does not set `LastPlayed`

**Severity:** High
**Files:**
- `backend/internal/integrations/enrichers.go:760-771` (TracearrEnricher.Enrich)
- `backend/internal/integrations/tracearr.go:76-86` (TracearrHistoryItem struct)

**Description:**
The TracearrEnricher sets `item.PlayCount` and `item.WatchedByUsers` but does not set `item.LastPlayed`. This is the only watch-data enricher that omits `LastPlayed`. The root cause is that `TracearrHistoryItem` (the struct mapping Tracearr's API response) has no timestamp field — it has `DurationMs` but no `startedAt`, `completedAt`, or `date` field.

For comparison:
- `TautulliEnricher` sets `LastPlayed` from `entry.Date`
- `BulkWatchEnricher` sets `LastPlayed` from `wd.LastPlayed`
- `JellystatEnricher` sets `LastPlayed` from `wd.LastPlayed`

**Impact:** Items enriched only by Tracearr always have `LastPlayed == nil`. This causes:
- The `lastplayed` rule field evaluates as "never played"
- The `LastPlayedFactor` scoring factor treats them as unwatched (maximum deletion score)
- The `staleContentDays` analytics misclassifies them

**Plan claim:** The Tracearr integration plan (`20260326T0024Z`) is marked complete.

**Remediation:** Check whether the Tracearr Public API provides session timestamps. If so, add the field to `TracearrHistoryItem` and set `item.LastPlayed` in the enricher. If not, document this as a known Tracearr API limitation.

---

### F-06: `DetectEnrichment()` omits Tracearr

**Severity:** High
**Files:**
- `backend/internal/services/integration.go:763-781` (DetectEnrichment switch)
- `backend/internal/services/rules.go:253-261` (EnrichmentPresence struct)
- `backend/internal/services/rules.go:312-340` (appendEnrichmentFieldDefs)

**Description:**
`DetectEnrichment()` has cases for Tautulli, Seerr, Plex/Jellyfin/Emby, Sonarr, and Jellystat — but no case for Tracearr. The `EnrichmentPresence` struct has `HasTautulli`, `HasSeerr`, `HasMedia`, `HasSonarr`, `HasJellystat` but no `HasTracearr`. The rule field gating logic in `appendEnrichmentFieldDefs()` checks `p.HasTautulli || p.HasMedia` for `playcount` and `lastplayed` fields.

**Impact:** When Tracearr is the user's only watch-data source (no Plex, Jellyfin, Emby, or Tautulli), Tracearr enriches `PlayCount` and `WatchedByUsers` during the poll cycle, but the rule editor does not show `playcount` or `lastplayed` fields. Users cannot create rules based on the data Tracearr provides. If they also have a media server connected, the `HasMedia` flag masks this gap.

**Remediation:** Add `HasTracearr` to `EnrichmentPresence`, add a Tracearr case to `DetectEnrichment()`, and update `appendEnrichmentFieldDefs()` to include Tracearr in the watch-data gate.

---

### F-07: OpenAPI notification channel schemas are stale

**Severity:** High
**Files:**
- `docs/reference/api/openapi.yaml:3544-3615` (NotificationChannel / NotificationChannelInput schemas)
- `backend/internal/db/models.go:327-347` (actual NotificationConfig model)
- `backend/internal/db/migrations/00010_notification_tiers.sql` (migration that changed the schema)

**Description:**
The OpenAPI spec still documents the pre-migration-00010 boolean fields (`onCycleDigest`, `onError`, `onModeChanged`, `onServerStarted`, `onThresholdBreach`, `onUpdateAvailable`, `onApprovalActivity`, `onIntegrationStatus`). The actual model now uses `NotificationLevel` (string: "all", "important", "minimal") plus 8 `Override*` nullable integer fields.

**Impact:** Any API client generated from the OpenAPI spec will send/expect the wrong fields. External integrations relying on the documented API cannot correctly configure notification channels.

---

### F-08: OpenAPI `LibraryHistory` field name capitalization mismatch

**Severity:** High
**Files:**
- `docs/reference/api/openapi.yaml:3161-3188` (schema with capitalized names)
- `backend/internal/db/models.go:80-88` (model with json tags in lowercase camelCase)

**Description:**
The OpenAPI schema documents `LibraryHistory` properties as `ID`, `Timestamp`, `TotalCapacity`, etc. with a note saying "No json tags on this struct." However, the Go struct does have json tags producing lowercase camelCase: `id`, `timestamp`, `totalCapacity`, etc.

**Impact:** API clients generated from OpenAPI expect `Timestamp` but receive `timestamp`.

---

### F-09: OpenAPI ghost `reason` field

**Severity:** High
**Files:**
- `docs/reference/api/openapi.yaml:3356-3358` (AuditLogEntry.reason)
- `docs/reference/api/openapi.yaml:3414` (ApprovalQueueItem.reason)
- `backend/internal/db/models.go:278-292` (actual AuditLogEntry model — no `Reason` field)

**Description:**
The OpenAPI spec documents a `reason` field on `AuditLogEntry` and `ApprovalQueueItem`. No such field exists in the Go models. The actual data is in `ScoreDetails` (JSON-encoded scoring factors).

**Impact:** API clients expecting `reason` will receive empty/missing values.

---

### F-10: 10 API endpoints missing from OpenAPI spec

**Severity:** Medium
**Files:**
- `docs/reference/api/openapi.yaml` (missing endpoints)
- `backend/routes/` (registered endpoints)

**Missing endpoints:**

| Endpoint | Route Handler |
|----------|--------------|
| `GET /integrations/health` | `routes/integrations.go:165` |
| `POST /approval-queue/group/approve` | `routes/approval.go:153` |
| `POST /approval-queue/group/reject` | `routes/approval.go:183` |
| `PATCH /preferences/engine` | `routes/preferences.go:88` |
| `PATCH /preferences/sunset` | `routes/preferences.go:119` |
| `PATCH /preferences/content` | `routes/preferences.go:141` |
| `PATCH /preferences/advanced` | `routes/preferences.go:163` |
| `POST /sunset-queue/refresh-labels` | `routes/sunset.go:180` |
| `POST /sunset-queue/refresh-posters` | `routes/sunset.go:203` |
| `GET /preview` `force` query param | `routes/preview.go:14` |

---

### F-11: OpenAPI `info.version` stale at `2.0.0`

**Severity:** Medium
**Files:**
- `docs/reference/api/openapi.yaml:4` (version field)

**Description:**
The schema is at v3 (per migration 00006 updating `schema_info`). The OpenAPI version should reflect the current API version.

---

### F-12: OpenAPI `action` enum includes nonexistent `dry_run`

**Severity:** Medium
**Files:**
- `docs/reference/api/openapi.yaml:3365` (action enum)
- `backend/internal/db/models.go:258` (ActionDryDelete constant)

**Description:**
The OpenAPI `AuditLogEntry.action` enum lists `dry_run`, but the code only produces `dry_delete`. No `dry_run` constant exists.

---

### F-13: `approval_returned_to_pending` — no frontend handler

**Severity:** Medium
**Files:**
- `backend/internal/events/types.go:475` (event definition)
- `backend/internal/services/approval.go:847` (published)
- Frontend — zero references

**Description:**
When a dry-deleted approval item is returned to pending status, the `approval_returned_to_pending` event is published and persisted to the activity log. However, no frontend composable or page subscribes to this event. The notification dispatch service also does not handle it.

**Impact:** The transition happens silently — no notification to external channels and no real-time frontend update. The approval queue composable only refreshes on `engine_complete`, causing a potentially long desync.

---

### F-14: `data_reset` — no frontend handler

**Severity:** Medium
**Files:**
- `backend/internal/events/types.go:884` (event definition)
- `backend/internal/services/data.go:108` (published)

**Description:**
After clearing all scraped data via `POST /data/reset`, the event is published but the frontend has no SSE handler. The dashboard retains stale data until the user manually refreshes the page.

---

### F-15: `settings_imported` — no frontend handler

**Severity:** Medium
**Files:**
- `backend/internal/events/types.go:156` (event definition)
- `backend/internal/services/backup.go:454,1419` (published)

**Description:**
After importing settings from a backup, the settings page and dashboard don't auto-refresh. The user must manually reload.

---

### F-16: `analytics_updated` — no frontend handler

**Severity:** Medium
**Files:**
- `backend/internal/events/types.go:850` (event definition)
- `backend/internal/services/preview.go:160,193` (published alongside `preview_updated`)

**Description:**
The `analytics_updated` event fires simultaneously with `preview_updated` when the preview cache refreshes. The frontend subscribes to `preview_updated` but not `analytics_updated`. If the library page's analytics tab used this event for real-time refresh, users would see updated dead/stale content analytics without manual refresh.

---

### F-17: `deadContentMinDays` / `staleContentDays` — no settings UI

**Severity:** Medium
**Files:**
- `backend/internal/db/models.go:109-110` (fields with defaults 90/180)
- `backend/internal/services/settings.go:76-77` (partial update support)
- `frontend/app/types/api.ts:70-71` (TypeScript type definition)
- No settings component references these fields

**Description:**
The `PreferenceSet` model includes `DeadContentMinDays` (default 90) and `StaleContentDays` (default 180), and the settings service supports partial updates of these values via API. However, no frontend settings page exposes them. Users cannot configure what "dead" or "stale" means through the web UI — they must use direct API calls.

---

### F-18: `EngineRunStats.ExecutionMode` — per-disk-group mode tracking TODO

**Severity:** Medium
**Files:**
- `backend/internal/poller/poller.go:216` (TODO comment)
- `backend/internal/db/models.go:310` (ExecutionMode field)

**Description:**
With per-disk-group modes (introduced in the sunset feature), a single engine run can span multiple modes (e.g., dry-run for group A, approval for group B, sunset for group C). However, `EngineRunStats.ExecutionMode` stores only the `DefaultDiskGroupMode` — a single string. The TODO at `poller.go:216` acknowledges this.

**Impact:** Engine run history in the UI shows the default mode, not the actual modes used per disk group.

---

### F-19: `IntegrationService.Update()` — test-only code

**Severity:** Low
**Files:**
- `backend/internal/services/integration.go:510` (definition)

**Description:**
Full-replace `Update()` method exists alongside `PartialUpdate()`. Only `PartialUpdate()` is used by route handlers. `Update()` is only called from tests.

---

### F-20: `NotificationChannelService.Update()` — test-only code

**Severity:** Low
**Files:**
- `backend/internal/services/notification_channel.go:41` (definition)

**Description:**
Same pattern as F-19. Full-replace `Update()` exists but only `PartialUpdate()` is used in production.

---

### F-21: `SunsetService.CancelAllForDiskGroup()` — test-only code

**Severity:** Low
**Files:**
- `backend/internal/services/sunset.go:495` (definition)

**Description:**
Per-disk-group sunset cancellation exists but is unused in production. `CancelAll()` (which cancels all groups) is the only method called from route handlers.

---

### F-22: `MappingService.DeleteStale()` — test-only code

**Severity:** Low
**Files:**
- `backend/internal/services/mapping.go:149` (definition)

**Description:**
`GarbageCollect()` (called from the daily cron) handles stale cleanup internally. `DeleteStale()` is a public method that duplicates this functionality and is only called from tests.

---

### F-23: `rules.vue` stale `prefs` reactive object

**Severity:** Low
**Files:**
- `frontend/app/pages/rules.vue:96-112` (reactive object + fetch function)
- `frontend/app/pages/rules.vue:266` (onMounted call)

**Description:**
The `prefs` reactive object is declared, populated by `fetchPreferences()`, and fetched on mount, but it is never read anywhere in the template, computed properties, or passed to child components. The properties `defaultDiskGroupMode`, `tiebreakerMethod`, `logLevel`, and `auditLogRetentionDays` are set but never consumed.

---

### F-24: Unused type definitions in `api.ts`

**Severity:** Low
**Files:**
- `frontend/app/types/api.ts:212-219` (DashboardStats — never imported)
- `frontend/app/types/api.ts:298-313` (MetricsHistoryResponse, LibraryHistoryRow — never imported)

**Description:**
Three interfaces are defined in the API types file but never imported by any component, page, or composable.

---

### F-25: Unused function exports

**Severity:** Low
**Files:**
- `frontend/app/utils/format.ts:14` (`formatTime()` — never imported outside tests)
- `frontend/app/composables/useEChartsDefaults.ts:180` (`generatePalette()` — exported, never used)
- `frontend/app/composables/useEChartsDefaults.ts:197` (`chart2Color` — exported from composable, never destructured)
- `frontend/app/composables/useThemeColors.ts:95,104` (`chart4Color` — exported, never imported)
- `frontend/app/composables/useThemeColors.ts:106` (`refresh()` — deprecated no-op, never called)
- `frontend/app/composables/useVersion.ts:15,98` (`apiCommit` — exported, never consumed)
- `frontend/app/composables/useAutoSave.ts:85` (`showSaveStatus()` — exported, only used internally)
- `frontend/app/composables/useMotionPresets.ts:40,46,58` (`slideInLeft`, `slideInRight`, `slideDownFromTop` — never used)

**Description:**
10 exported functions/values across frontend files are never consumed by any caller.

---

### F-26: Orphaned locale keys (notification center UI)

**Severity:** Low
**Files:**
- `frontend/app/locales/en.json` (keys under `nav.notifications`, `nav.noNotifications`, `nav.markAllRead`, `nav.clearAll`)

**Description:**
Locale keys for an in-app notification center (bell icon with dropdown) exist in the English locale file but no corresponding UI components exist. These suggest a planned feature that was either removed or never implemented.

Additional orphaned keys:
- `settings.typeDeleteToConfirm` — data reset dialog uses hardcoded strings
- `settings.clearDataDesc` — same
- `settings.deletionSafetyDesc` — `deletionSafetyExplain` is used instead

---

### F-27: `album` media type unused

**Severity:** Low
**Files:**
- `backend/internal/db/migrations/00001_v2_baseline.sql:176` (CHECK constraint includes `album`)
- No Go code references `"album"` as a media type

**Description:**
The `approval_queue_items.media_type` CHECK constraint includes `album` alongside `movie, show, season, episode, artist, book`. However, no Go code produces or consumes `"album"` as a media type value. Lidarr integration produces `artist`-level items, not album-level. This suggests album-level tracking was planned for Lidarr but not implemented.

---

### F-28: `EngineRunStats` `created_at` column without Go field

**Severity:** Low
**Files:**
- `backend/internal/db/migrations/00001_v2_baseline.sql:241` (SQL column)
- `backend/internal/db/models.go:301-313` (Go struct — no `CreatedAt` field)

**Description:**
The `engine_run_stats` table has a `created_at` column with `DEFAULT CURRENT_TIMESTAMP`, but the `EngineRunStats` Go struct has no corresponding field. SQLite's default still populates the column, but it is inaccessible to Go code. The `RunAt` field serves the same purpose.

---

### F-29: OpenAPI Settings tag undefined

**Severity:** Low
**Files:**
- `docs/reference/api/openapi.yaml:938-940` (uses tag `[Settings]`)
- `docs/reference/api/openapi.yaml:33-77` (tags list — no `Settings` tag defined)

**Description:**
Settings export/import endpoints reference a `Settings` tag that is not defined in the OpenAPI tags list. The closest defined tag is `Settings Backup`.

---

## Remediation Priority

### Immediate (bugs and broken features) — COMPLETE

| Finding | Action | Status |
|---------|--------|--------|
| F-04 | Add `nativeIDSearchers` cleanup to `Unregister()` and `Clear()` | Done |
| F-01 | Wire `UpdateSavedOverlay()` into the sunset save flow; also updated `UpdateAll()` to route saved items through `UpdateSavedOverlay()` | Done |
| F-02 | Wire `MigrateLabel()` into the sunset preferences save path via `SunsetLabelMigrator` interface + adapter pattern | Done |

### Near-term (functional gaps) — COMPLETE

| Finding | Action | Status |
|---------|--------|--------|
| F-05 + F-06 | Added `Date` field to `TracearrHistoryItem`, `LastPlayed` tracking in enricher, `HasTracearr` to `EnrichmentPresence`, Tracearr case in `DetectEnrichment()`, and updated rule field gating | Done |
| F-03 | Removed dead `AlertSunsetActivity` constant and all rendering code (trigger label, color, Apprise priority). Sunset events stay in generic categories per notification tiers design. | Done |
| F-13 | Added `approval_returned_to_pending` handler in approval queue composable + dashboard activity feed | Done |
| F-14 | Added `data_reset` SSE handler on dashboard to trigger full refresh | Done |
| F-15 | Added `settings_imported` to dashboard activity feed with icon/color mapping + SSE handler for full refresh | Done |
| F-17 | Added Content Analytics card in SettingsGeneral.vue with select dropdowns for `deadContentMinDays` and `staleContentDays` | Done |

### OpenAPI spec update (batch) — COMPLETE

| Findings | Action | Status |
|----------|--------|--------|
| F-07 | Updated `NotificationChannel` and `NotificationChannelInput` schemas to use `notificationLevel` + 8 `override*` nullable bool fields | Done |
| F-08 | Fixed `LibraryHistory` field names from capitalized to camelCase | Done |
| F-09 | Removed ghost `reason` field from both `AuditLogEntry` and `ApprovalQueueItem` | Done |
| F-10 | Added 10 missing endpoint definitions (integrations health, group approve/reject, preference patches, sunset label/poster refresh) | Done |
| F-11 | Updated `info.version` from `2.0.0` to `3.0.0` | Done |
| F-12 | Removed nonexistent `dry_run` from action enum, leaving only `deleted` and `dry_delete` | Done |
| F-29 | Fixed Settings tag references from `[Settings]` to `[Settings Backup]` | Done |

### Backlog (cleanup) — MOSTLY COMPLETE

| Finding | Action | Status |
|---------|--------|--------|
| F-16 | Wired `analytics_updated` SSE event to library page `fetchFilterData()` | Done |
| F-18 | Added `DiskGroupModes` JSON field to `EngineRunStats` + migration 00011 + poller wiring via `SetDiskGroupModes()` | Done |
| F-19 through F-22 | Annotated test-only methods with doc comments clarifying production code uses alternative methods | Done |
| F-23 | Removed stale `prefs` reactive object, `fetchPreferences()`, and unused imports from `rules.vue` | Done |
| F-24 | Removed 3 unused type definitions from `api.ts`: `DashboardStats`, `LibraryHistoryRow`, `MetricsHistoryResponse` | Done |
| F-25 | Removed 10 unused exports: `formatTime`, `generatePalette`, `chart2Color`, `chart4Color`, `refresh`, `apiCommit`, `showSaveStatus`, `slideInLeft`, `slideInRight`, `slideDownFromTop`. Also removed dead helpers `hexToHSL`, `hslToHex`, and associated `formatTime` tests. | Done |
| F-26 | Removed 7 orphaned locale keys: `nav.notifications`, `nav.noNotifications`, `nav.markAllRead`, `nav.clearAll`, `settings.deletionSafetyDesc`, `settings.clearDataDesc`, `settings.typeDeleteToConfirm` | Done |
| F-27 | Removed unused `album` from `approval_queue` CHECK constraint via migration 00012 + baseline update | Done |
| F-28 | Added `CreatedAt time.Time` field to `EngineRunStats` Go struct to expose the SQL `created_at` column | Done |
