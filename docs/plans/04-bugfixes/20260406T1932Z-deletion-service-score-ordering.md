# fix: DeletionService must sort queued items by score before draining

**Status:** ✅ Complete
**Priority:** Critical (data loss in wrong order)
**Date:** 2026-04-06

## Problem

Items in the deletion queue are processed in FIFO (insertion) order. The
DeletionService is a dumb pipe — it trusts callers to enqueue items in the
correct order. This means every code path that calls `QueueDeletion()` must
independently implement correct score-based ordering, and if any one of them
gets it wrong, items die in the wrong order.

**Observed in production:** Disk group 3 is in sunset mode. `Escalate()`
ordered items by `deletion_date ASC` / `created_at ASC` (time-based), causing
items with scores 0.64–0.72 to be deleted while 8 items with scores 1.30–1.69
remained in the library.

## Root Cause

Sunset mode stores candidates in a DB table (`sunset_queue`) with their score
preserved. But when items are re-queried for processing — via `ProcessExpired()`,
`Escalate()` step 1, or `Escalate()` step 2 — the queries order by time instead
of score, discarding the engine's scoring.

| Query | Previous `ORDER BY` | Fixed `ORDER BY` |
|-------|---------------------|------------------|
| `GetExpired()` | `deletion_date ASC` | `score DESC` |
| `Escalate()` step 1 (expired) | `deletion_date ASC` | `score DESC` |
| `Escalate()` step 2 (non-expired) | `created_at ASC` | `score DESC` |

Other modes (auto, dry-run, approval) are not affected because they go directly
from the engine's score-sorted candidate list into DeletionService or the
approval queue — no intermediate DB re-query.

## Fix

### Layer 1: Fix sunset re-query ordering (root cause)

Changed the three `ORDER BY` clauses in `sunset.go` to `score DESC`.

### Layer 2: DeletionService sorts before draining (defense-in-depth)

Added `sort.SliceStable` by `Score` descending in `drainAll()` before the drain
loop. This makes DeletionService a priority queue regardless of insertion order,
so even if a future caller enqueues items wrong, they'll still be processed
highest-score-first.

### Tests

- `TestDeletionService_DrainAll_SortsByScoreDescending`: enqueues 4 items in
  ascending score order, verifies they process descending.
- `TestGetExpired_OrdersByScoreDescending`: verifies the DB query returns items
  score-descending.
- Updated `TestEscalate_OrderAndTargetBytes` seed data to use explicit scores.

## Steps

- [x] Step 1: Create branch `fix/deletion-service-score-ordering`
- [x] Step 2: Add score-based sort to `drainAll()` in `deletion.go`
- [x] Step 3: Fix `Escalate()` queries in `sunset.go`
- [x] Step 4: Fix `GetExpired()` query in `sunset.go`
- [x] Step 5: Add `TestDeletionService_DrainAll_SortsByScoreDescending`
- [x] Step 6: Add `TestGetExpired_OrdersByScoreDescending` + update `TestEscalate_OrderAndTargetBytes`
- [x] Step 7: `make ci` — all stages passed
- [x] Step 8: Commit
