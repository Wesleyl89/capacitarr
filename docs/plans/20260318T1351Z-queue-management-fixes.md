# Queue Management, Force-Delete, Stale Data, and Queue Status Fixes

**Status:** 🔲 Not Started  
**Created:** 2026-03-18T13:51Z  
**Scope:** Backend services, poller, routes, frontend composables and components

## Background

A deep-dive analysis identified four interconnected issues in the approval/deletion queue system:

1. **Items stuck in approval queue** — no way to remove pending/rejected items when deletions are disabled
2. **Force-delete returns 409 in dry-run mode** — should simulate (dry-delete) instead of blocking
3. **Stale data after deletions** — deleted items still show in Library page and Deletion Priority card until next engine cycle
4. **No queue status indicators** — Library/Deletion Priority views have no visual link to approval or deletion queue state

These issues compound each other: force-delete items can also get stuck (issue 2b), stale data makes it hard to tell what's been processed (issue 3), and the lack of queue indicators means users can't see what's pending (issue 4).

## Phase 1: Approval Queue Dismiss/Clear API

Add the ability for users to remove items from the approval queue without approving or rejecting them.

### Step 1.1: Add `Dismiss` method to `ApprovalService`

**File:** `backend/internal/services/approval.go`

Add a `Dismiss(entryID uint)` method that:
- Finds the entry by ID
- Validates status is `pending` or `rejected` (not `approved` — those are mid-deletion)
- Deletes the row from `approval_queue`
- Publishes a new `ApprovalDismissedEvent` via the event bus
- Returns an error if the entry is not found or not in a dismissable state

### Step 1.2: Add `ApprovalDismissedEvent` to event types

**File:** `backend/internal/events/types.go`

Add a new event type:
```go
type ApprovalDismissedEvent struct {
    EntryID   uint   `json:"entryId"`
    MediaName string `json:"mediaName"`
    MediaType string `json:"mediaType"`
}
```

Implement `EventType() string` returning `"approval_dismissed"` and `EventMessage() string`.

### Step 1.3: Add `DELETE /api/v1/approval-queue/:id` route

**File:** `backend/routes/approval.go`

Add a DELETE handler that:
- Parses the entry ID from the URL
- Calls `reg.Approval.Dismiss(entryID)`
- Returns 200 on success, 404 if not found, 400 if not in dismissable state

### Step 1.4: Add `POST /api/v1/approval-queue/clear` route

**File:** `backend/routes/approval.go`

Add a POST handler that:
- Calls the existing `reg.Approval.ClearQueue()` (already clears pending+rejected items)
- Returns 200 with `{ "cleared": count }`

This allows users to clear the queue on demand rather than only when disk drops below threshold.

### Step 1.5: Add route tests

**File:** `backend/routes/approval_test.go`

Add test cases for:
- Dismissing a pending item (200)
- Dismissing a rejected item (200)
- Dismissing an approved item (400 — not dismissable)
- Dismissing a non-existent item (404)
- Clearing the queue (200 with count)

### Step 1.6: Add service tests

**File:** `backend/internal/services/approval_test.go`

Add test cases for:
- `Dismiss()` on a pending item
- `Dismiss()` on a rejected item
- `Dismiss()` on an approved item (should return error)
- `Dismiss()` on a non-existent item (should return error)
- Verify `ApprovalDismissedEvent` is published

### Step 1.7: Update frontend `useApprovalQueue` composable

**File:** `frontend/app/composables/useApprovalQueue.ts`

- Add `dismissGroup(group: ApprovalGroup)` method that calls `DELETE /api/v1/approval-queue/:id` for each `auditId`
- Add `clearQueue()` method that calls `POST /api/v1/approval-queue/clear`
- Subscribe to `approval_dismissed` SSE event to refresh the queue
- Return `dismissGroup` and `clearQueue` from the composable

### Step 1.8: Update `ApprovalQueueCard` component

**File:** `frontend/app/components/ApprovalQueueCard.vue`

The existing action buttons follow a consistent pattern: icon-only `UiButton variant="ghost"` at `h-7 w-7 p-0` with contextual hover colors:
- Approve: green `CheckIcon`
- Snooze: amber `AlarmClockIcon`
- Undo snooze: `Undo2Icon`

Add a dismiss button that matches this pattern:
- Use `XIcon` (already imported from lucide-vue-next in the file) with muted/destructive styling
- Style: `text-muted-foreground hover:text-destructive hover:bg-destructive/10 dark:hover:bg-destructive/20`
- Position: after the snooze button as the third action icon
- Apply to both grid view (season popover rows) and list view (group action area)
- Include for both pending and snoozed sections
- Add a "Clear All" ghost button in the card header bar (near the existing badge area), using `Trash2Icon` with muted styling, only visible when there are pending or snoozed items
- Wire dismiss to `dismissGroup()` / `dismissSeason()` and clear to `clearQueue()` from the composable

### Step 1.9: Update activity event handling

**File:** `backend/internal/events/activity_persister.go`

Ensure the `ApprovalDismissedEvent` is persisted to the activity log so it shows in the dashboard activity feed.

### Step 1.10: Run `make ci`

Verify all linting, tests, and security checks pass.

---

## Phase 2: Force-Delete Dry-Run Simulation

Allow force-delete to work in dry-run mode and when `DeletionsEnabled=false`, flowing through to the DeletionService for simulation.

### Step 2.1: Add `ForceDryRun` flag to `DeleteJob`

**File:** `backend/internal/services/deletion.go`

Add a `ForceDryRun bool` field to the `DeleteJob` struct. This flag tells the DeletionService to skip the actual deletion even if `DeletionsEnabled=true`.

### Step 2.2: Update `DeletionService.processJob()` to check `ForceDryRun`

**File:** `backend/internal/services/deletion.go`

In `processJob()`, change the dry-delete check from:
```go
if !deletionsEnabled {
```
to:
```go
if !deletionsEnabled || job.ForceDryRun {
```

This ensures force-delete items queued with `ForceDryRun=true` always get dry-deleted.

### Step 2.3: Remove route handler guards for force-delete

**File:** `backend/routes/approval.go`

In the `POST /force-delete` handler:
- Remove the `DeletionsEnabled` guard (line 113-115)
- Remove the `ExecutionMode == "dry-run"` guard (line 116-118)
- The route should accept force-delete staging in any mode

### Step 2.4: Update `processForceDeletes()` in the poller

**File:** `backend/internal/poller/evaluate.go`

In `processForceDeletes()`:
- Remove the `DeletionsEnabled` early-return guard (line 238-241)
- Determine if force-dry-run is needed: `forceDryRun := !prefs.DeletionsEnabled || prefs.ExecutionMode == "dry-run"`
- Pass `ForceDryRun: forceDryRun` in the `DeleteJob` struct when queueing
- Always call `RemoveForceDelete(id)` after successful queueing (existing behavior, but now happens in all cases rather than being short-circuited)

### Step 2.5: Update approval route — remove `DeletionsEnabled` guard on approve

**File:** `backend/routes/approval.go`

In the `POST /approval-queue/:id/approve` handler:
- Remove the `DeletionsEnabled` guard (line 52-54)
- Instead, determine `forceDryRun` and pass it through `ExecuteApprovalDeps` or a new parameter
- Update `ApprovalService.ExecuteApproval()` to accept and propagate the `ForceDryRun` flag to the `DeleteJob`

### Step 2.6: Update `ExecuteApproval()` to support `ForceDryRun`

**File:** `backend/internal/services/approval.go`

- Add `ForceDryRun bool` to `ExecuteApprovalDeps`
- Pass it through to `QueueDeletion()` in the `DeleteJob`

### Step 2.7: Update route tests for force-delete

**File:** `backend/routes/approval_test.go`

- Remove/update tests that expect 409 for dry-run mode or DeletionsEnabled=false
- Add tests verifying force-delete returns 200 in dry-run mode
- Add tests verifying force-delete returns 200 when DeletionsEnabled=false

### Step 2.8: Update service tests for deletion

**File:** `backend/internal/services/deletion_test.go`

- Add test case for `processJob()` with `ForceDryRun=true` and `DeletionsEnabled=true` — should dry-delete, not actually delete
- Add test case for `processJob()` with `ForceDryRun=false` and `DeletionsEnabled=false` — should dry-delete (existing behavior)

### Step 2.9: Run `make ci`

Verify all linting, tests, and security checks pass.

---

## Phase 3: Stale Data After Deletions

Ensure the frontend removes deleted items from the Library page and Deletion Priority card in real-time.

### Step 3.1: Add client-side filtering on `deletion_success` in `usePreview.ts`

**File:** `frontend/app/composables/usePreview.ts`

Subscribe to the `deletion_success` SSE event. When received, filter the deleted item out of the local `items` ref:

```typescript
on('deletion_success', (data) => {
  const event = data as { mediaName: string; mediaType: string };
  items.value = items.value.filter(
    (item) => !(item.item.title === event.mediaName && item.item.type === event.mediaType)
  );
});
```

Register in `onMounted()`, unregister in `onUnmounted()`.

### Step 3.2: Add `deletion_dry_run` filtering in `usePreview.ts`

**File:** `frontend/app/composables/usePreview.ts`

Also filter on `deletion_dry_run` events, since dry-deleted items should also be marked/removed from the preview (they've been "processed" even if not actually deleted). Use a visual differentiation — either remove or add a `dryDeleted` flag.

Decision needed: Should dry-deleted items be removed from the list or marked with a badge? Recommend **removal** for consistency — the audit log is the authoritative record of dry-deletions.

### Step 3.3: Add batch reconciliation on `deletion_batch_complete`

**File:** `frontend/app/composables/usePreview.ts`

Subscribe to `deletion_batch_complete` and call `refresh(true)` to re-fetch authoritative data from the server after all deletions for a cycle are complete.

### Step 3.4: Verify Library page and dashboard reflect changes

**Files:** `frontend/app/pages/library.vue`, `frontend/app/pages/index.vue`

Both consume `usePreview()` — verify that reactive updates propagate correctly when `items.value` is mutated or replaced. The Library page uses `items` directly from `usePreview()`, so filtering should propagate automatically.

The dashboard's deletion priority display (if it uses preview data) should also reflect changes.

### Step 3.5: Run `make ci`

Verify all linting, tests, and security checks pass.

---

## Phase 4: Queue Status Indicators

Add visual indicators in the Library table and Deletion Priority views showing queue state for each item.

### Step 4.1: Add `QueueStatus` field to `engine.EvaluatedItem`

**File:** `backend/internal/engine/score.go`

Add new fields to the `EvaluatedItem` struct:
```go
QueueStatus     string `json:"queueStatus,omitempty"`     // "", "pending", "approved", "force_delete", "deleting"
ApprovalQueueID *uint  `json:"approvalQueueId,omitempty"` // for linking to approval actions
```

### Step 4.2: Add `EnrichWithQueueStatus()` to `PreviewService`

**File:** `backend/internal/services/preview.go`

Add a method that:
- Queries the approval queue for all items with `status IN ('pending', 'approved')`
- Builds a lookup map: `map[mediaName+mediaType] → {status, forceDelete, id}`
- Checks `DeletionService.CurrentlyDeleting()` for the active deletion
- For each `EvaluatedItem`, sets `QueueStatus` and `ApprovalQueueID` from the lookup

### Step 4.3: Add `ApprovalQueueReader` interface to `PreviewService`

**File:** `backend/internal/services/preview.go`

Define a new interface for the approval queue dependency:
```go
type ApprovalQueueReader interface {
    ListQueue(status string, limit int) ([]db.ApprovalQueueItem, error)
}
```

Add it as a dependency via `SetDependencies()`.

### Step 4.4: Add `DeletionStateReader` interface to `PreviewService`

**File:** `backend/internal/services/preview.go`

Define a new interface for the deletion service dependency:
```go
type DeletionStateReader interface {
    CurrentlyDeleting() string
}
```

Add it as a dependency via `SetDependencies()`.

### Step 4.5: Call `EnrichWithQueueStatus()` in `buildPreview()` and `SetPreviewCache()`

**File:** `backend/internal/services/preview.go`

After building the preview result, call `EnrichWithQueueStatus()` to annotate items with queue state before returning/caching.

### Step 4.6: Wire dependencies in `services.Registry`

**File:** `backend/internal/services/registry.go`

Update `SetDependencies()` to pass `ApprovalService` and `DeletionService` to `PreviewService`.

### Step 4.7: Update `EvaluatedItem` TypeScript type

**File:** `frontend/app/types/api.ts`

Add to `EvaluatedItem`:
```typescript
queueStatus?: 'pending' | 'approved' | 'force_delete' | 'deleting';
approvalQueueId?: number;
```

### Step 4.8: Add queue status badges to `LibraryTable`

**File:** `frontend/app/components/LibraryTable.vue`

For each item row, render a badge based on `item.queueStatus`:
- `pending` → amber badge "Pending Approval"
- `approved` → green badge "Approved"
- `force_delete` → red badge "Force Delete"
- `deleting` → animated badge "Deleting..."

### Step 4.9: Add real-time "deleting" status via SSE

**File:** `frontend/app/composables/usePreview.ts`

Subscribe to `deletion_progress` SSE events. When `currentItem` matches an item in the preview list, set its `queueStatus` to `"deleting"` locally.

### Step 4.10: Add service tests

**File:** `backend/internal/services/preview_test.go`

Add test cases for `EnrichWithQueueStatus()`:
- Item in pending approval queue → `queueStatus: "pending"`
- Item in approved state → `queueStatus: "approved"`
- Force-delete item → `queueStatus: "force_delete"`
- Currently deleting item → `queueStatus: "deleting"`
- Item not in any queue → `queueStatus: ""`

### Step 4.11: Run `make ci`

Verify all linting, tests, and security checks pass.

---

## File Change Summary

### Backend files modified:
- `backend/internal/services/approval.go` — Add `Dismiss()`, update `ExecuteApproval()` for `ForceDryRun`
- `backend/internal/services/approval_test.go` — Add dismiss tests
- `backend/internal/services/deletion.go` — Add `ForceDryRun` to `DeleteJob`, update `processJob()`
- `backend/internal/services/deletion_test.go` — Add ForceDryRun tests
- `backend/internal/services/preview.go` — Add `EnrichWithQueueStatus()`, new interfaces
- `backend/internal/services/preview_test.go` — Add queue status enrichment tests
- `backend/internal/services/registry.go` — Wire new dependencies
- `backend/internal/engine/score.go` — Add `QueueStatus` and `ApprovalQueueID` fields
- `backend/internal/events/types.go` — Add `ApprovalDismissedEvent`
- `backend/internal/events/activity_persister.go` — Handle new event
- `backend/internal/poller/evaluate.go` — Update `processForceDeletes()` guards
- `backend/routes/approval.go` — Add DELETE and clear routes, remove force-delete guards
- `backend/routes/approval_test.go` — Update and add tests

### Frontend files modified:
- `frontend/app/types/api.ts` — Add `queueStatus`, `approvalQueueId` to `EvaluatedItem`
- `frontend/app/composables/usePreview.ts` — Add SSE handlers for deletion events
- `frontend/app/composables/useApprovalQueue.ts` — Add `dismissGroup()`, `clearQueue()`, SSE handler
- `frontend/app/components/ApprovalQueueCard.vue` — Add dismiss/clear buttons
- `frontend/app/components/LibraryTable.vue` — Add queue status badges
