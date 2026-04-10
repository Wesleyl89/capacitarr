# Context Propagation for Integration HTTP Layer

**Status:** Planned
**Priority:** Medium (reliability/shutdown safety)
**Estimated Effort:** L (3-4 days)
**Origin:** Brittleness & Rearchitecture Audit, Category 4 (Findings 4.1, 4.2)

---

## Summary

Thread `context.Context` through the entire integration HTTP call chain — from the poller's shutdown signal down through integration clients, `DoAPIRequest` variants, and the enrichment pipeline. This enables cancellation of in-flight HTTP requests on graceful shutdown and adds per-operation timeouts to prevent runaway paginated fetches from blocking the process indefinitely.

Currently, all integration HTTP calls use hardcoded `context.Background()` with no cancellation path. When `pollerInstance.Stop()` is called during shutdown, it stops the timer loop but any running `poll()` call — which may be mid-way through dozens of paginated API calls — runs to completion or waits for the 30-second HTTP client timeout on each individual call. A full Jellyfin library sync could take minutes to drain.

---

## Current State

### DoAPIRequest Variants (all hardcode `context.Background()`)

| Function | File | Line | Signature |
|----------|------|------|-----------|
| `DoAPIRequest` | `integrations/httpclient.go` | L69 | `func DoAPIRequest(url, headerKey, headerValue string) ([]byte, error)` |
| `DoAPIRequestWithBody` | `integrations/httpclient.go` | L117 | `func DoAPIRequestWithBody(method, url string, body []byte, headerKey, headerValue string) error` |
| `DoMultipartUpload` | `integrations/httpclient.go` | L162 | `func DoMultipartUpload(url string, imageData []byte, fieldName, fileName string, extraHeaders map[string]string) error` |
| `DoAPIRequestWithHeaders` | `integrations/httpclient.go` | L213 | `func DoAPIRequestWithHeaders(method, url string, body []byte, headers map[string]string) error` |

### Inline `context.Background()` in Integration Code

| File | Line | Description |
|------|------|-------------|
| `sonarr.go` | L205 | `http.NewRequestWithContext(context.Background(), "DELETE", ...)` in `DeleteMediaItem` |
| `arr_helpers.go` | L240 | `http.NewRequestWithContext(context.Background(), "DELETE", ...)` in `arrSimpleDelete` |

### Integration Clients — Zero Context Acceptance

All 12 integration client structs (`arrBaseClient`, `SonarrClient`, `RadarrClient`, `LidarrClient`, `ReadarrClient`, `PlexClient`, `JellyfinClient`, `EmbyClient`, `TautulliClient`, `JellystatClient`, `TracearrClient`, `SeerrClient`) have zero methods that accept `context.Context`.

### Paginated Fetchers (Sequential Multi-Call, No Bailout)

| Client | Method | File | Pattern |
|--------|--------|------|---------|
| `JellyfinClient` | `GetBulkWatchDataForUser` | `jellyfin.go` | 2 paginated passes (L148-284) |
| `JellyfinClient` | `GetFavoritedItems` | `jellyfin.go` | Paginated (L297-326) |
| `JellyfinClient` | `GetItemIDToTMDbIDMap` | `jellyfin.go` | Paginated (L447-476) |
| `JellyfinClient` | `GetLabelMemberships` | `jellyfin.go` | Paginated (L607-639) |
| `JellyfinClient` | `GetLabelNames` | `jellyfin.go` | Paginated (L656-689) |
| `JellyfinClient` | `GetTMDbToItemIDMap` | `jellyfin.go` | Paginated (L815-844) |
| `JellyfinClient` | `GetCollectionMemberships` | `jellyfin.go` | Per-BoxSet child fetch (L567-589) |
| `EmbyClient` | *(same as Jellyfin)* | `emby.go` | Mirror of all Jellyfin paginated methods |
| `TautulliClient` | `getAllHistory` | `tautulli.go` | Pages through all history, 1000/request (L282-317) |
| `TracearrClient` | `GetWatchHistory` | `tracearr.go` | Pages through movies then episodes (L92-139) |
| `SeerrClient` | `GetRequestedMedia` | `seerr.go` | Pages through all requests (L87-131) |
| `PlexClient` | `fetchMediaItems` | `plex.go` | Iterates all library sections (L164-187) |

### Enrichment Pipeline — No Context

- `EnrichmentPipeline.Run(items []MediaItem)` at `enrichment_pipeline.go:83` — no context parameter
- `Enricher` interface `Enrich(items []MediaItem) error` at `enrichment_pipeline.go:36` — no context parameter
- All enrichers in `enrichers.go` (809 lines) implement the context-less interface

### Poller — No Context

- Uses `done chan struct{}` for shutdown signaling (`poller.go:79`), not `context.Context`
- `Stop()` method (`poller.go:129-131`) closes the `done` channel
- Integration client methods called without context in `fetch.go` (L100, L164, L280-281)

### Graceful Shutdown

`main.go` (L526-555) calls `pollerInstance.Stop()` during shutdown, which stops the timer loop but **does not** cancel any in-flight HTTP requests. The HTTP server gets a 10-second shutdown deadline but integration API calls get no deadline beyond the per-request 30-second HTTP client timeout.

---

## Design

### Context Flow

```
main.go shutdown signal
  |
  v
context.WithCancel(context.Background()) ── pollerCtx
  |
  ├── poller.Run(pollerCtx)
  |     |
  |     ├── poll(pollerCtx)
  |     |     |
  |     |     ├── fetchAllIntegrations(pollerCtx)
  |     |     |     |
  |     |     |     ├── client.TestConnection(ctx)
  |     |     |     ├── client.GetMediaItems(ctx)
  |     |     |     ├── client.GetDiskSpace(ctx)
  |     |     |     └── ... all integration methods
  |     |     |           |
  |     |     |           └── DoAPIRequest(ctx, ...) → http.NewRequestWithContext(ctx, ...)
  |     |     |
  |     |     └── enrichmentPipeline.Run(ctx, items)
  |     |           |
  |     |           └── enricher.Enrich(ctx, items)
  |     |                 |
  |     |                 └── client.GetWatchData(ctx) → DoAPIRequest(ctx, ...)
  |     |
  |     └── evaluateDiskGroup(pollerCtx, ...)  [for cancellation during evaluation]
  |
  └── pollerCancel()  ← called during graceful shutdown
```

### Timeout Strategy

| Scope | Timeout | Mechanism |
|-------|---------|-----------|
| Per HTTP request | 30s (existing) | `http.Client.Timeout` (unchanged) |
| Per integration operation | 2 minutes | `context.WithTimeout` wrapping each top-level client call |
| Total poll cycle | 10 minutes | `context.WithTimeout` wrapping `poll()` |
| Graceful shutdown | Immediate cancellation | `pollerCancel()` cancels the context tree |

### Interface Changes

The `MediaSource`, `DiskReporter`, `CollectionNameFetcher`, and `Enricher` interfaces all need a `context.Context` first parameter on their methods. This is a mechanical signature change with no behavioral change for callers that pass `context.Background()` during the transition.

---

## Implementation Steps

### Phase 1: DoAPIRequest Foundation

1. **Add `context.Context` as the first parameter to all 4 `DoAPIRequest` variants** in `integrations/httpclient.go`. Replace `context.Background()` with the caller-provided context in `http.NewRequestWithContext()`. Update all call sites to pass `context.TODO()` initially (compile-fix pass).

2. **Update the 2 inline `context.Background()` sites** in `sonarr.go:205` and `arr_helpers.go:240` to accept context from the calling method.

3. **Add context parameter to `arrBaseClient.doRequest()`** — this is the internal dispatch method used by all *arr clients. Threading context here propagates to `DoAPIRequest` automatically.

4. **Add context parameter to each integration client's `doRequest()` method** — Plex, Jellyfin, Emby, Tautulli, Jellystat, Tracearr, Seerr each have their own `doRequest`. Update all call sites within each client to forward the context.

5. **Tests:** Verify all existing integration tests still pass. No new tests needed yet — this phase is signature-only.

### Phase 2: Integration Client Methods

6. **Add `context.Context` as the first parameter to every public method on every integration client.** This is a mechanical change affecting ~80+ method signatures across 12 client files. Each method forwards context to `doRequest(ctx, ...)`.

7. **Update the `MediaSource`, `DiskReporter`, and `CollectionNameFetcher` interfaces** to include `context.Context` on all methods. Update the interface assertion call sites.

8. **Add context checking in paginated fetch loops.** In every paginated method (Jellyfin, Emby, Tautulli, Tracearr, Seerr, Plex), add `ctx.Err()` check at the top of each loop iteration:
   ```go
   for startIndex < totalCount {
       if err := ctx.Err(); err != nil {
           return nil, fmt.Errorf("fetch cancelled: %w", err)
       }
       // ... existing pagination logic
   }
   ```

9. **Tests:** Update all integration test call sites to pass `context.Background()`. Add one test per paginated client verifying that a cancelled context aborts mid-pagination.

### Phase 3: Enrichment Pipeline

10. **Add `context.Context` to the `Enricher` interface:** `Enrich(ctx context.Context, items []MediaItem) error`

11. **Add `context.Context` to `EnrichmentPipeline.Run()`:** `Run(ctx context.Context, items []MediaItem) ([]MediaItem, error)`. Add `ctx.Err()` check between each enricher stage.

12. **Update all enricher implementations** in `enrichers.go` to accept and forward context to their integration client calls.

13. **Tests:** Update enrichment pipeline tests. Add a test verifying pipeline short-circuits when context is cancelled between stages.

### Phase 4: Poller Integration

14. **Replace `done chan struct{}` with `context.Context` in the `Poller` struct.** Store a `ctx context.Context` and `cancel context.CancelFunc`. `Stop()` calls `cancel()` instead of closing a channel.

15. **Pass context through `poll()` → `fetchAllIntegrations()` → integration client calls.** The context flows from the poller's stored context through `prepareContext()` and into every fetch goroutine.

16. **Wrap each `poll()` invocation with a total cycle timeout:** `pollCtx, pollCancel := context.WithTimeout(p.ctx, 10*time.Minute)`. This prevents a single poll cycle from running forever if an integration API is unresponsive.

17. **Pass context through `poll()` → `evaluateDiskGroup()`** and down through evaluation helpers. This allows shutdown to cancel evaluation mid-flight.

18. **Pass context through `poll()` → `enrichmentPipeline.Run(ctx, ...)`** so enrichment is also cancellable.

19. **Update `main.go`** to create a `pollerCtx, pollerCancel := context.WithCancel(context.Background())` and pass it to `poller.New(pollerCtx, ...)`. Call `pollerCancel()` in the shutdown goroutine before or instead of `pollerInstance.Stop()`.

20. **Tests:** Update poller tests. Add a test verifying that cancelling the poller context causes an in-flight poll to return promptly.

### Phase 5: Per-Operation Timeouts

21. **Wrap top-level integration calls with per-operation timeouts.** In `fetchAllIntegrations()`, wrap each integration's `GetMediaItems` / `GetDiskSpace` / `TestConnection` call with `context.WithTimeout(ctx, 2*time.Minute)`. This caps any single integration from blocking the entire cycle.

22. **Document timeout values** in code comments and in the architecture docs. The three-tier timeout model (30s per HTTP request, 2m per integration operation, 10m per poll cycle) should be clearly explained.

23. **Tests:** Add a test verifying that a per-operation timeout fires and returns an error without blocking the entire poll cycle.

### Phase 6: Cleanup & Verification

24. **Audit for remaining `context.Background()` in production code.** The following are expected to remain:
    - `services/deletion.go:157` — deletion worker has its own `context.WithCancel` (already correct)
    - `services/schema.go` — startup-only, runs before poller exists
    - `services/version.go` — background version check, independent of poller
    - `notifications/httpclient.go` — notification dispatch is independent
    - `services/poster_overlay.go:408` — poster download uses `http.DefaultClient` with no timeout (separate bug, not in scope)

25. **Run `make ci`** to verify all lints, tests, and security checks pass.

26. **Update the parent audit plan** (`07-audits/20260407T1412Z-brittleness-and-rearchitecture-audit.md`) to reference this plan file and mark Category 4 as addressed.

---

## Out of Scope

- **Per-integration configurable timeouts** (Category 9 of the audit) — separate concern, tracked separately
- **Poster download timeout fix** (`poster_overlay.go` using `http.DefaultClient`) — separate bug
- **Notification dispatch context** — notification goroutines are independent of the poller lifecycle and already have their own 10-second timeouts

## Risk Assessment

- **Zero behavioral change for users** — all existing functionality works identically; context propagation only adds cancellation capability
- **High mechanical risk** — ~80+ method signatures change across 12+ files; typos or missed call sites cause compile errors (caught immediately)
- **Low regression risk** — the refactor is additive (adding a parameter) with no logic changes; existing tests verify behavior is preserved
- **Phase ordering matters** — each phase must compile and pass tests before proceeding to the next; do not combine phases
