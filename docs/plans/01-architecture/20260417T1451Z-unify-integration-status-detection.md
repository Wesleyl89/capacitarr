# Unify Integration Status Detection and Notification

**Status:** ✅ Complete
**Created:** 2026-04-17T14:51Z
**Revised:** 2026-04-19T04:11Z
**Scope:** Backend — `internal/services/`, `internal/poller/`, `main.go`

## Problem

Integration connection testing is performed in four separate code paths, each with inconsistent side effects:

| Path | File | Publishes failure event | Updates DB sync status | Notifies recovery tracker | Publishes recovery event | Sends notification |
|------|------|------------------------|------------------------|--------------------------|--------------------------|-------------------|
| Poller cycle | `poller/fetch.go:100` | ❌ | ✅ | ✅ | ✅ | ❌ |
| Startup self-test | `main.go:396` | ✅ | ❌ | ❌ | ❌ | ✅ (1st failure) |
| Manual UI test | `routes/integrations.go:160` → `IntegrationService.TestConnection()` | ✅ | ✅ (if saved) | ✅ (if saved) | ✅ (if saved) | ✅ (1st failure) |
| Manual sync | `IntegrationService.SyncAll():950` | ❌ | ❌ | ❌ | ❌ | ❌ |

### Issues

1. **Manual test sends notifications** — The user is actively viewing the result in the UI. Sending a Discord/Apprise "Integration Down" notification is redundant noise, especially during troubleshooting.
2. **Startup self-test bypasses DB tracking** — Passes `integrationID=nil` so `ConsecutiveFailures` is never incremented, recovery tracker is never notified, and `LastError` is never persisted.
3. **No consecutive-failure gating** — Notifications fire on the first failure regardless of `ConsecutiveFailures`. Transient blips (container restart, network hiccup, brief API timeout) trigger false alarms.
4. **SyncAll bypasses everything** — The manual sync endpoint tests connections but performs no status tracking, event publishing, or recovery notification.
5. **Poller never notifies** — Even after many consecutive failures, the poller path never sends an "Integration Down" notification. Users relying on notifications (not the web UI) would never learn an integration is down from the poller path alone.
6. **Health checking is coupled to the poller cadence** — The poller interval is tuned for the engine evaluation workload (5-10 min), not for health monitoring. The `RecoveryService` (30s-5min backoff probing) and startup self-test both exist as band-aids for this cadence mismatch. Health checking should run on its own independent schedule.

### Architectural root cause

Health checking is not a poller concern — it's a cross-cutting concern that multiple components need but none should own. The poller's job is to fetch data, evaluate disk usage, and queue deletions. It currently performs connection testing only because it needs to know which integrations are reachable before fetching data, but that's a query ("is this healthy?") not a responsibility ("maintain health state").

## Design

### Principles

1. **Dedicated `IntegrationHealthService`** — Replaces the current `RecoveryService` and absorbs all health checking responsibilities into a single service with its own independent ticker.
2. **Single source of truth for health state** — The health service owns `ConsecutiveFailures`, `LastError`, `LastSync`, in-memory health state, backoff probing, and notification decisions. No other component writes health state.
3. **Multiple sources of signal** — The health service's own ticker is the primary health check source, but any component that talks to an integration (poller data fetches, SyncAll) can report failures back via `ReportFailure()`. The health service is the authority; other components are informants.
4. **Manual test is notification-silent** — The UI test button tests the connection and returns the result. It does NOT update health state or publish notification events. The user is staring at the result.
5. **Consecutive-failure threshold** — "Integration Down" notifications fire only after N consecutive failures (constant, initially 3). Recovery notifications fire on the first success after a failure streak.
6. **Poller consults, doesn't test** — The poller asks the health service which integrations are healthy and fetches data from those only. It reports unexpected data-fetch failures back to the health service.

### What gets absorbed from `RecoveryService`

The current `RecoveryService` (`internal/services/recovery.go`) does:
- In-memory tracking of failing integrations (`map[uint]*recoveryState`)
- Exponential backoff probing (30s base, doubling to 5min cap, 15s tick period)
- DB seeding from `LastError` on startup
- `TrackFailure()` / `TrackRecovery()` called by `IntegrationService.UpdateSyncStatus()`
- `HealthStatus()` API for `GET /integrations/health`
- `IntegrationRecoveryAttemptEvent` publishing for frontend SSE

All of this moves into `IntegrationHealthService` with the addition of:
- Proactive health checking of ALL integrations (not just failing ones)
- Threshold-gated `IntegrationTestFailedEvent` publishing
- `ReportFailure()` / `ReportSuccess()` for external signal sources
- `IsHealthy(id)` / `HealthyIDs()` for poller consultation

### What gets removed

- **`RecoveryService`** — fully absorbed into `IntegrationHealthService`
- **`RecoveryTracker` interface** — replaced by `HealthReporter` interface on `IntegrationHealthService`
- **Startup self-test in `main.go`** — the health service's first tick handles this naturally
- **Connection testing in `fetchAllIntegrations`** — poller consults health service instead
- **`IntegrationService.UpdateSyncStatus()` recovery tracker notification** — `IntegrationHealthService` owns all state updates directly
- **`IntegrationService.UpdateSyncStatusDirect()`** — absorbed into health service's internal DB writes
- **`IntegrationService.PublishRecoveryIfNeeded()`** — absorbed into health service's recovery detection

### New service: `IntegrationHealthService`

```go
// IntegrationHealthService monitors integration connectivity, tracks health
// state, and publishes notification events with consecutive-failure gating.
// It replaces RecoveryService and absorbs all health-related concerns that
// were previously scattered across the poller, startup self-test, and
// IntegrationService.
type IntegrationHealthService struct {
    integrationSvc *IntegrationService
    bus            *events.EventBus

    mu       sync.Mutex
    states   map[uint]*healthState   // integrationID → state (ALL enabled integrations)
    done     chan struct{}
    stopOnce sync.Once
}
```

### Health state per integration

```go
type healthState struct {
    IntegrationID       uint
    IntegrationType     string
    Name                string
    URL                 string
    APIKey              string
    Healthy             bool
    LastError           string
    ConsecutiveFailures int
    LastCheck           time.Time
    NextCheck           time.Time       // healthy: regular interval; failing: backoff
    NotificationSent    bool            // true once "down" notification was sent at threshold
}
```

### Key methods

```go
// Start seeds state from DB, runs initial health check, then begins the
// background ticker. Replaces both RecoveryService.Start() and the startup
// self-test goroutine in main.go.
func (h *IntegrationHealthService) Start()

// Stop signals the background goroutine to exit.
func (h *IntegrationHealthService) Stop()

// ReportFailure is called by external components (poller data fetch, SyncAll)
// when an I/O operation against an integration fails. Equivalent to a health
// check failure — updates state, increments counter, may trigger notification.
func (h *IntegrationHealthService) ReportFailure(id uint, err error)

// ReportSuccess is called by external components when an I/O operation
// succeeds, confirming the integration is reachable.
func (h *IntegrationHealthService) ReportSuccess(id uint)

// IsHealthy returns whether a specific integration is currently healthy.
// Used by the poller to decide whether to fetch data from an integration.
func (h *IntegrationHealthService) IsHealthy(id uint) bool

// HealthyIDs returns the set of integration IDs currently considered healthy.
func (h *IntegrationHealthService) HealthyIDs() map[uint]bool

// HealthStatus returns the API-facing health snapshot for GET /integrations/health.
// Replaces RecoveryService.HealthStatus().
func (h *IntegrationHealthService) HealthStatus() []IntegrationHealthEntry
```

### Tick behavior

- **Healthy integrations:** checked every `healthCheckInterval` (2 minutes default).
- **Failing integrations:** checked with linear backoff (15s base, +15s per step, 60s cap). Schedule: 15s → 30s → 45s → 60s → 60s... This replaces the `RecoveryService`'s exponential backoff (30s base, doubling to 5min cap). Linear is preferred because these are private LAN endpoints with no rate limiting — exponential backoff protects shared resources, which doesn't apply here. The 60s cap ensures recovery detection is always faster than the poller cadence.
- **First tick on startup:** runs immediately, replacing the startup self-test.

### Notification logic (inside the health service)

On failure:
1. Increment `ConsecutiveFailures` in memory and DB.
2. If `ConsecutiveFailures == connectionFailureThreshold` (3), publish `IntegrationTestFailedEvent` and set `NotificationSent = true`.
3. If `ConsecutiveFailures > threshold` and `NotificationSent` is already true, don't publish again.
4. Switch integration to backoff schedule.

On success (after failures):
1. If `ConsecutiveFailures > 0`, publish `IntegrationRecoveredEvent`.
2. Reset `ConsecutiveFailures` to 0 in memory and DB, set `NotificationSent = false`.
3. Switch integration back to regular health check interval.

On success (no prior failures):
1. Update `LastCheck`, no events.

### Poller data-fetch failure reporting

When the poller's data fetch fails for an integration the health service considers healthy:

```go
// In fetchAllIntegrations, after a media fetch fails:
items, err := source.GetMediaItems()
if err != nil {
    healthSvc.ReportFailure(integrationID, err)
    continue
}

// After a successful fetch:
healthSvc.ReportSuccess(integrationID)

// Same for disk fetches
```

This ensures the health service's state stays current even between its own tick intervals.

### EvaluationContext `brokenTypes`

The poller currently builds `brokenTypes` from connection test failures in `fetchAllIntegrations` and passes them to `engine.NewEvaluationContext()`. With the health service, this changes to:

```go
// In prepareContext(), replace the connection-test-derived brokenTypes
// with the health service's current state:
brokenIDs := healthSvc.UnhealthyIDs()
brokenTypes := healthSvc.UnhealthyTypes()
evalCtx := engine.NewEvaluationContext(configTypes, brokenTypes)
```

The scoring engine already handles broken types correctly via `IsIntegrationBroken()` — it skips factors whose required integration is broken rather than penalizing items. This behavior is preserved.

## Phase 1: Create `IntegrationHealthService`

### Step 1.1: Create `internal/services/health.go`

- **File:** New file `internal/services/health.go`
- **Contents:**
  - `const connectionFailureThreshold = 3`
  - `const healthCheckInterval = 2 * time.Minute`
  - `healthState` struct (see Design section above)
  - `IntegrationHealthService` struct
  - `NewIntegrationHealthService(integrationSvc *IntegrationService, bus *events.EventBus) *IntegrationHealthService`
  - Linear backoff logic: `min(backoffBase * consecutiveFailures, backoffCap)` where `backoffBase = 15s`, `backoffCap = 60s`. Replaces the exponential backoff from `recoveryState.nextBackoff()`.
  - `seed()` — loads all enabled integrations from DB, populates `states` map. Failing integrations (non-empty `LastError`) start on backoff schedule; healthy ones start on regular interval.

### Step 1.2: Implement `Start()` and `Stop()`

- **File:** `internal/services/health.go`
- **`Start()`:** calls `seed()`, runs initial health check of all integrations (replacing startup self-test), then starts background ticker goroutine.
- **Ticker:** 15-second tick period (same as current `RecoveryService`). On each tick, collect integrations due for checking (`time.Now() >= NextCheck`), probe them outside the lock.
- **`Stop()`:** closes done channel, same pattern as `RecoveryService.Stop()`.

### Step 1.3: Implement `checkIntegration()`

- **File:** `internal/services/health.go`
- **Logic:** Creates integration client, calls `TestConnection()`, then:
  - On failure: call `recordFailure(id, err)` — increments counter, updates DB via `IntegrationService.UpdateSyncStatusDirect()`, publishes `IntegrationTestFailedEvent` if threshold reached, publishes `IntegrationRecoveryAttemptEvent` (for SSE), schedules next check with backoff.
  - On success: call `recordSuccess(id)` — publishes `IntegrationRecoveredEvent` if previously failed, resets counter, updates DB, publishes `IntegrationRecoveryAttemptEvent` (for SSE), schedules next check at regular interval.
- **Note:** Uses `IntegrationService.UpdateSyncStatusDirect()` (not `UpdateSyncStatus()`) to avoid the current recovery tracker notification loop. The health service owns the full state update path.

### Step 1.4: Implement `ReportFailure()` and `ReportSuccess()`

- **File:** `internal/services/health.go`
- **`ReportFailure(id, err)`:** Acquires lock, calls `recordFailure()` with the same logic as a tick-discovered failure. This is how the poller and SyncAll report unexpected I/O errors.
- **`ReportSuccess(id)`:** Acquires lock, calls `recordSuccess()`. This is how the poller confirms an integration is reachable after a successful data fetch.
- **Edge case:** If `id` is not in the `states` map (integration added after last seed), reload its config from DB and add it.

### Step 1.5: Implement query methods

- **File:** `internal/services/health.go`
- **`IsHealthy(id uint) bool`** — returns `states[id].Healthy` under lock.
- **`HealthyIDs() map[uint]bool`** — returns set of healthy integration IDs.
- **`UnhealthyTypes() []string`** — returns integration types currently failing (for `EvaluationContext.BrokenIntegrationTypes`).
- **`HealthStatus() []IntegrationHealthEntry`** — returns API-facing snapshot of all tracked integrations' health. Replaces `RecoveryService.HealthStatus()`. Returns entries for ALL integrations (healthy and failing), unlike the old `RecoveryService` which only returned failing ones.
- **`TrackedCount() int`** — returns count of integrations being monitored (all enabled, not just failing).

### Step 1.6: Unit tests for `IntegrationHealthService`

- **File:** New file `internal/services/health_test.go`
- **Test cases:**
  1. `seed()` — loads healthy and failing integrations from DB; failing ones get backoff schedule, healthy ones get regular interval.
  2. `recordFailure()` — first failure: counter=1, no notification. Second: counter=2, no notification. Third (threshold): counter=3, `IntegrationTestFailedEvent` published exactly once. Fourth: counter=4, no additional event.
  3. `recordSuccess()` after failures — `IntegrationRecoveredEvent` published, counter reset to 0, `NotificationSent` reset.
  4. `recordSuccess()` with no prior failures — no events, counter stays 0.
  5. `ReportFailure()` — same as `recordFailure()`, accessible from external callers.
  6. `ReportSuccess()` — same as `recordSuccess()`, accessible from external callers.
  7. `IsHealthy()` / `HealthyIDs()` / `UnhealthyTypes()` — correct state after mixed successes and failures.
  8. `HealthStatus()` — returns all integrations, not just failing ones.
  9. Linear backoff calculation — verify schedule: 15s, 30s, 45s, 60s, 60s, 60s... (replaces `TestRecoveryState_NextBackoff`).
- **Pattern:** In-memory SQLite, subscribe to event bus, assert event types and counts. Follow existing patterns from `recovery_test.go`.

## Phase 2: Wire into Registry and remove `RecoveryService`

### Step 2.1: Add `IntegrationHealthService` to `Registry`

- **File:** `internal/services/registry.go`
- **Action:** Replace `Recovery *RecoveryService` with `Health *IntegrationHealthService` on the `Registry` struct.
- **Wiring in `NewRegistry()`:**
  ```go
  reg.Health = NewIntegrationHealthService(reg.Integration, bus)
  ```
- **Remove:** `NewRecoveryService()` construction, `SetRecoveryTracker()` call.

### Step 2.2: Remove `RecoveryTracker` interface from `IntegrationService`

- **File:** `internal/services/integration.go`
- **Remove:** `RecoveryTracker` interface definition, `recoveryTracker` field on `IntegrationService`, `SetRecoveryTracker()` method.
- **Simplify `UpdateSyncStatus()`:** Remove the recovery tracker notification calls (lines 842-847 and 865-867). The health service manages its own state directly. `UpdateSyncStatus()` becomes a pure DB update method.
- **Keep `UpdateSyncStatusDirect()`:** Still needed by the health service for direct DB writes without side effects.

### Step 2.3: Remove `RecoveryService`

- **File:** Delete `internal/services/recovery.go`
- **File:** Delete `internal/services/recovery_test.go`
- **Note:** All functionality has been absorbed into `IntegrationHealthService`. The backoff logic, seed logic, probe logic, and health status API are all reimplemented in the new service.

### Step 2.4: Update `GET /integrations/health` route

- **File:** `routes/integrations.go`
- **Current (line 165):** `entries := reg.Recovery.HealthStatus()`
- **Change to:** `entries := reg.Health.HealthStatus()`

### Step 2.5: Update startup sequence in `main.go`

- **File:** `main.go`
- **Remove:** The startup self-test goroutine (lines 378-413). The health service's `Start()` handles initial health checks.
- **Remove:** `reg.Recovery.Start()` (line 376).
- **Add:** `reg.Health.Start()` after registry construction and `InitVersion()`.
- **Add:** `reg.Health.Stop()` in the shutdown sequence.

### Step 2.6: Update `cron.go` if it references `Recovery`

- **File:** `internal/jobs/cron.go`
- **Action:** Check for any references to `reg.Recovery` and update to `reg.Health`.

### Step 2.7: Update all imports and references

- **Action:** Search for all references to `RecoveryService`, `RecoveryTracker`, `Recovery`, `recoveryState` across the codebase and update or remove them.

## Phase 3: Migrate poller to consult health service

### Step 3.1: Remove connection testing from `fetchAllIntegrations`

- **File:** `internal/poller/fetch.go`
- **Remove:** The entire "Parallel connection tests" section (lines 74-136): the goroutine pool that calls `conn.TestConnection()`, the sequential result processing, the `brokenSet` accumulation.
- **Change `fetchAllIntegrations` signature:** Add `healthSvc *IntegrationHealthService` parameter (or access via `services.Registry`).
- **Add:** Filter media sources and disk reporters by health status before fetching:
  ```go
  healthyIDs := healthSvc.HealthyIDs()
  // Only fetch from healthy integrations
  for id, source := range registry.MediaSources() {
      if !healthyIDs[id] {
          slog.Debug("Skipping unhealthy integration", "component", "poller", "integrationID", id)
          continue
      }
      // ... existing fetch logic
  }
  ```
- **Remove:** `brokenTypes` field from `fetchResult` struct.
- **Remove:** `connTestResult` struct.

### Step 3.2: Add failure/success reporting to data fetches

- **File:** `internal/poller/fetch.go`
- **Media fetch failures:** When `source.GetMediaItems()` returns an error, call `healthSvc.ReportFailure(id, err)`.
- **Media fetch successes:** When `source.GetMediaItems()` succeeds, call `healthSvc.ReportSuccess(id)`.
- **Disk fetch failures:** When `reporter.GetDiskSpace()` or `reporter.GetRootFolders()` returns an error, call `healthSvc.ReportFailure(id, err)`.
- **Disk fetch successes:** When disk fetch succeeds, call `healthSvc.ReportSuccess(id)`.
- **Note:** This ensures the health service's state stays current even between its own tick intervals. If an integration was healthy 1 minute ago but fails during a data fetch, the health service learns about it immediately rather than waiting for its next tick.

### Step 3.3: Update `EvaluationContext` construction in `prepareContext`

- **File:** `internal/poller/poller.go`
- **Current (line 307):** `evalCtx := engine.NewEvaluationContext(configTypes, fetched.brokenTypes)`
- **Change to:** `evalCtx := engine.NewEvaluationContext(configTypes, reg.Health.UnhealthyTypes())`
- **Effect:** The scoring engine's broken-type awareness now comes from the health service's persistent state rather than the poller's per-cycle connection test results.

### Step 3.4: Update poller tests

- **File:** `internal/poller/fetch_test.go`, `internal/poller/evaluate_test.go`, `internal/poller/poller_test.go`
- **Action:** Update tests that previously relied on connection testing within `fetchAllIntegrations`. Tests may need to:
  - Provide a mock or real `IntegrationHealthService` to `fetchAllIntegrations`.
  - Remove assertions about `brokenTypes` on `fetchResult`.
  - Add assertions about `healthSvc.ReportFailure()` / `ReportSuccess()` calls if testing data-fetch error paths.

## Phase 4: Silence manual test notifications

### Step 4.1: Remove event publishing from manual `TestConnection` path

- **File:** `internal/services/integration.go`
- **Current:** `TestConnection()` (line 274) calls `s.testClient()` which calls `PublishTestFailure()` / `PublishTestSuccess()`.
- **Change:** Replace `s.testClient(intType, url, conn.TestConnection)` with a direct `conn.TestConnection()` call. Handle the result inline:
  ```go
  if err := conn.TestConnection(); err != nil {
      result = TestConnectionResult{Success: false, Error: err.Error()}
  } else {
      result = TestConnectionResult{Success: true, Message: "Connection successful"}
  }
  ```
- **Remove:** The sync status + recovery tracker block (lines 297-312). The manual test should not update DB health state — that's the health service's job. The UI already shows the result directly.
- **Effect:** Pressing "Test" in the UI never sends Discord/Apprise notifications and never mutates health state. It's a pure diagnostic.

### Step 4.2: Evaluate `testClient`, `PublishTestFailure`, `PublishTestSuccess` for removal

- After Step 4.1, check whether `testClient()`, `PublishTestFailure()`, and `PublishTestSuccess()` have any remaining callers.
- `PublishTestFailure` is still called by `IntegrationHealthService.recordFailure()` (Phase 1).
- `PublishTestSuccess` may have no callers — if so, remove it and `IntegrationTestEvent` to avoid dead code.
- `testClient()` may have no callers — if so, remove it.

### Step 4.3: Unit test that manual test does NOT publish notification events

- **File:** `internal/services/integration_test.go`
- **Test:** Call `TestConnection()` with a failing integration. Subscribe to the event bus. Assert that no `IntegrationTestFailedEvent` is published. Assert that DB health state (`ConsecutiveFailures`, `LastError`) is NOT modified.

## Phase 5: Migrate `SyncAll` to report to health service

### Step 5.1: Update `SyncAll` to report connection results

- **File:** `internal/services/integration.go`
- **Change `SyncAll` signature or dependency:** `SyncAll` needs access to the health service. Options:
  - Add `healthSvc *IntegrationHealthService` as a lazily-wired dependency on `IntegrationService` (similar to `diskGroups`).
  - Or accept it as a parameter.
- **After connection test (line 950):** Call `healthSvc.ReportFailure(cfg.ID, connErr)` on failure, `healthSvc.ReportSuccess(cfg.ID)` on success.
- **Keep:** The existing early-continue on failure (lines 951-954) — `SyncAll` still skips disk/media fetches for broken integrations.

### Step 5.2: Unit test `SyncAll` health reporting

- **File:** `internal/services/integration_test.go`
- **Verify:** Existing `TestIntegrationService_SyncAll_*` tests still pass. Add a case confirming that `ReportFailure` is called when `SyncAll` encounters a connection failure, and `ConsecutiveFailures` is incremented in the health service state.

## Phase 6: Run full CI and smoke test

### Step 6.1: Run `make ci`

- **Command:** `make ci` (from `capacitarr/` directory)
- **Expected:** All lint, test, and security checks pass.

### Step 6.2: Manual smoke test

- Start the Docker container with a misconfigured integration (bad URL).
- Verify: Health service detects failure on its first tick (within ~2 minutes).
- Verify: No "Integration Down" notification on 1st or 2nd health check failure.
- Verify: 3rd consecutive failure produces exactly one "Integration Down" notification.
- Verify: Fix the URL and wait — health service detects recovery, sends "Integration Recovered" notification.
- Verify: Pressing "Test" on a broken integration in the UI does NOT produce a notification or change health state.
- Verify: Poller skips data fetches for unhealthy integrations.
- Verify: If a healthy integration's data fetch fails, the health service learns about it immediately via `ReportFailure`.
- Verify: `GET /api/v1/integrations/health` returns all integrations (healthy and failing) with correct state.

## Files Modified

| File | Action |
|------|--------|
| `internal/services/health.go` | **New** — `IntegrationHealthService` |
| `internal/services/health_test.go` | **New** — tests for health service |
| `internal/services/recovery.go` | **Delete** — absorbed into health service |
| `internal/services/recovery_test.go` | **Delete** — tests migrated to `health_test.go` |
| `internal/services/registry.go` | Replace `Recovery` with `Health`, update wiring |
| `internal/services/integration.go` | Remove `RecoveryTracker` interface and field, simplify `UpdateSyncStatus()`, modify `TestConnection()` to stop publishing events and stop updating DB state, wire health service for `SyncAll`, potentially remove `testClient()`/`PublishTestSuccess()` |
| `internal/services/integration_test.go` | Update tests for silent manual test, update `SyncAll` tests |
| `internal/poller/fetch.go` | Remove connection testing section, add health-based filtering, add `ReportFailure`/`ReportSuccess` calls on data fetches |
| `internal/poller/poller.go` | Update `EvaluationContext` construction to use health service |
| `internal/poller/fetch_test.go` | Update for new fetch flow |
| `main.go` | Remove startup self-test goroutine, replace `Recovery.Start()` with `Health.Start()` |
| `internal/jobs/cron.go` | Update any `Recovery` references |
| `routes/integrations.go` | Update health endpoint to use `Health` service |
| `internal/events/types.go` | Potentially remove `IntegrationTestEvent` if unused |

## Risks

1. **Threshold delay on genuine outages** — A real outage won't notify until 3 failures. Worst case: up to 2 minutes for the first failure to be detected (healthy integration check interval), then 15s + 30s on the linear backoff for failures 2 and 3, totaling ~2 min 45s before notification. This is substantially faster than the old poller-coupled approach (~30 minutes) and faster than the original exponential backoff design (~3 min 30s).
2. **Additional network traffic** — Health checks and data fetches are no longer batched. Each integration gets hit by both the health tick (lightweight `TestConnection`) and the poller cycle (heavy data fetch). With 2-minute health checks and 5-10 minute poller intervals, this adds ~3-5 extra lightweight API calls per integration per poller cycle — negligible.
3. **SyncAll side effects** — `SyncAll` currently has no health side effects from connection testing. Adding `ReportFailure`/`ReportSuccess` means pressing "Sync" in the UI will update health state. This is correct behavior (sync is an automated-equivalent action, not a manual diagnostic), but it changes the existing contract.
4. **Stale health state at poller start** — The poller consults health state that may be up to 2 minutes old (healthy check interval) or up to 60 seconds old (failing integration backoff cap). An integration could recover between the last health check and the poller's data fetch, causing the poller to skip a reachable integration. This is mitigated by the poller's `ReportSuccess` calls — if a data fetch succeeds against an integration the health service thought was down, the health service immediately updates.
5. **Registry wiring complexity** — The `IntegrationHealthService` needs `IntegrationService` at construction time. `IntegrationService` does NOT need the health service (the `RecoveryTracker` dependency is removed). This eliminates the previous circular wiring concern.
