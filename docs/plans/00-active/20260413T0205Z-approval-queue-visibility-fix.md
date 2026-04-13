# Fix: Approval Queue Visibility Bug

**Status:** ✅ Complete
**Issue:** [#22 — Missing Approval queue](https://github.com/Ghent/capacitarr/issues/22)
**Reporter:** @dimitarmanov
**Branch:** `fix/approval-queue-visibility`
**Date:** 2026-04-13

## Problem

The Approval Queue card on the dashboard is invisible when per-disk-group modes are
set to "approval" because the visibility logic checks the **global
`defaultDiskGroupMode`** preference instead of the per-disk-group `DiskGroup.Mode`
fields.

Since v3.0, execution mode moved from a global setting to per-disk-group
(`DiskGroup.Mode`). The global `DefaultDiskGroupMode` preference is now only the
default for **newly auto-discovered** disk groups, but three places in the frontend
still gate on it as though it were the active mode:

1. `index.vue` line 454 — `approvalQueueVisible` computed
2. `useApprovalQueue.ts` line 83 — `fetchQueue()` early-return guard
3. `useApprovalQueue.ts` line 398 — SSE `refreshOnEvent` guard

### User Impact

When a user sets a disk group to "approval" mode via the Rules page:
- Items are scored and marked "Pending approval" in the library
- The dashboard shows "dry-run" mode badge (reads global default, not per-group)
- The `ApprovalQueueCard` is hidden (`v-if` evaluates to `false`)
- `fetchQueue()` early-returns and clears any queue state
- The user has no way to approve or reject pending deletions

## Root Cause Analysis

```
index.vue:454       approvalQueueVisible = isApprovalMode
                            ↓
useApprovalQueue.ts:76   isApprovalMode = executionMode === MODE_APPROVAL
                            ↓
useEngineControl.ts:60   executionMode = workerStats.defaultDiskGroupMode || 'dry-run'
                            ↓
Backend MetricsService   defaultDiskGroupMode = preferences.DefaultDiskGroupMode
                            ↓
                         Global default for NEW disk groups — NOT the active mode
```

The correct pattern already exists for `SunsetQueueCard` at `index.vue` line 338:
```vue
<SunsetQueueCard :has-sunset-mode="diskGroups.some((g) => g.mode === 'sunset')" />
```

## Fix Plan

### Phase 1: Remove incorrect guards from composable

**File:** `frontend/app/composables/useApprovalQueue.ts`

- [x] **Step 1.1:** Remove the `!isApprovalMode.value` early-return guard from `fetchQueue()` (lines 83–88). The backend `/api/v1/approval-queue` endpoint returns items regardless of mode — if the table is empty, the response is simply an empty array. The guard actively prevents the UI from showing items that exist in the database.

- [x] **Step 1.2:** Remove the `isApprovalMode.value` guard from the SSE `refreshOnEvent` handler (line 398). SSE events should always trigger a queue refresh so the UI stays in sync.

- [x] **Step 1.3:** Add a new exported computed `hasQueueItems` that returns `true` when any of the three queues (pending, snoozed, approved) contain items. This lets callers know the queue has content without needing to destructure all three lists.

### Phase 2: Fix dashboard visibility logic

**File:** `frontend/app/pages/index.vue`

- [x] **Step 2.1:** Change `approvalQueueVisible` to check per-disk-group modes (matching the `SunsetQueueCard` pattern) OR the presence of existing queue items:
  ```ts
  const approvalQueueVisible = computed(() =>
    diskGroups.value.some(g => g.mode === 'approval') || hasQueueItems.value
  );
  ```
  The `hasQueueItems` fallback ensures items are visible even if the user switches a disk group away from approval mode while items are still queued. Also moved `diskGroups` assignment before `fetchApprovalQueue()` in `fetchDashboardData()` so the computed evaluates correctly on initial load.

- [x] **Step 2.2:** Update the destructuring from `useApprovalQueue()` to include `hasQueueItems`.

- [x] **Step 2.3:** Update the comment on the `ApprovalQueueCard` line to reflect the new visibility logic.

### Phase 3: Fix README

**File:** `README.md`

- [x] **Step 3.1:** Clarify the Approval Queue feature description. Updated to: "Per-disk-group approval mode queues deletions for manual review on the dashboard before they execute".

### Phase 4: Update tests

**File:** `frontend/app/composables/useApprovalQueue.test.ts`

- [x] **Step 4.1:** Updated test "clears queues when not in approval mode" → "fetches queue items regardless of global execution mode". Now verifies that `fetchQueue()` always hits the API even when `executionModeRef` is `MODE_DRY_RUN`.

- [x] **Step 4.2:** Updated test "SSE handler does not fetchQueue when not in approval mode" → "SSE handler triggers fetchQueue regardless of global execution mode". Now verifies SSE handlers always trigger a refresh.

- [x] **Step 4.3:** Added `hasQueueItems` test suite with 4 cases: empty queues (false), pending items (true), approved items (true), snoozed items (true).

### Phase 5: Verify

- [x] **Step 5.1:** `make ci` passed — lint (Go + ESLint + Prettier + typecheck), tests (Go + Vitest), and security (govulncheck + Semgrep) all green.

### Phase 6–10: Dashboard mode indicators (cosmetic fixes)

All dashboard UI elements that used the global `defaultDiskGroupMode` were updated to
derive mode from per-disk-group modes. New computed helpers added to `index.vue`:
- `activeModes` — set of unique modes across all disk groups
- `effectiveMode` — "most aggressive" mode (auto > approval > sunset > dry-run)
- `allDryRun` — true only when ALL groups are dry-run
- `anyAutoMode` — true when any group is in auto mode

- [x] **Step 6.1:** Engine mode badge now uses `effectiveMode` instead of `engineExecutionMode`
- [x] **Step 7.1:** "Would Free" / "Freed" label now uses `anyAutoMode`
- [x] **Step 8.1:** "Active Delete" dry-run message now uses `allDryRun`
- [x] **Step 9.1:** Sparkline chart `isDryRunMode` now uses `allDryRun`
- [x] **Step 10.1:** `DeletionQueueCard` now accepts `effectiveMode` prop from parent

### Phase 11: Final verification

- [x] **Step 11.1:** `make ci` passed — lint, tests, security all green.

## Out of Scope

- **Dedicated approval page:** The approval queue is a dashboard card, not a standalone
  page with its own route. Adding a dedicated `/approval` page would be a feature
  enhancement, not a bug fix.

## Files Changed

| File | Change |
|------|--------|
| `frontend/app/composables/useApprovalQueue.ts` | Remove incorrect mode guards, add `hasQueueItems` |
| `frontend/app/composables/useApprovalQueue.test.ts` | Update tests for new behavior |
| `frontend/app/pages/index.vue` | Fix visibility logic, add per-disk-group mode computeds |
| `frontend/app/components/DeletionQueueCard.vue` | Accept `effectiveMode` prop for empty-state message |
| `README.md` | Clarify approval queue description |
