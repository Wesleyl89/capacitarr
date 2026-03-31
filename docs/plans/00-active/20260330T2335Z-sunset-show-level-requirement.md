# Sunset Mode: Require Show-Level Only for Sonarr Integrations

**Status:** ⛔ Superseded by `20260331T0231Z-sunset-virtual-show-level-override.md`  
**Priority:** Design Decision  
**Estimated Effort:** S (half day)

## Summary

Sunset mode currently operates on whatever media items the engine surfaces — including individual seasons. Sunsetting individual seasons creates a poor user experience: removing seasons 1-2 while keeping 3-4 leaves an unwatchable show. This plan proposes requiring `ShowLevelOnly=true` on all Sonarr integrations linked to a disk group before sunset mode can be enabled for that group.

## Problem Statement

The Sonarr integration has a `ShowLevelOnly` setting that controls whether the engine operates on whole shows or individual seasons. When `ShowLevelOnly=false`, the engine can surface individual seasons as sunset candidates. This leads to:

- Partial show removal (seasons 1-2 sunset while 3-4 remain)
- User confusion ("why is it removing half my show?")
- Nonsensical media library state (nobody starts a show from season 3)

Other modes (approval, auto-delete) have the same theoretical problem, but sunset mode makes it worse because users see the countdown on individual seasons and wonder about the logic.

## Current Behavior

- `ShowLevelOnly` exists on `IntegrationConfig` (Sonarr only)
- When enabled, `fetch.go` drops all `MediaTypeSeason` items before scoring
- Sunset mode evaluation (`evaluateSunsetMode`) has no awareness of `ShowLevelOnly`
- If `ShowLevelOnly=false`, seasons flow through to the sunset queue

## Design Options

### Option A: Explicit Validation (Recommended)

Add validation to `ValidateSunsetConfig()` (or the disk group mode change handler) that rejects sunset mode unless all Sonarr integrations linked to the disk group have `ShowLevelOnly=true`.

**Flow:**
1. User selects "Sunset" mode for a disk group
2. Backend checks all Sonarr integrations assigned to that disk group
3. If any have `ShowLevelOnly=false`, return a clear error:
   *"Sunset mode requires Show Level Only to be enabled on all Sonarr integrations linked to this disk group. This prevents partial show removal (e.g., sunsetting seasons 1-2 while keeping 3-4)."*
4. Frontend shows the validation error with a link to the integration settings

**Pros:**
- Explicit — user understands why and what to change
- Consistent — the setting means the same thing across all modes
- No silent behavioral differences

**Cons:**
- Forces a config change before enabling sunset
- Might surprise users who expected sunset to work on seasons

### Option B: Implicit Show-Level Filtering

When evaluating sunset mode, silently filter out season-level items regardless of the `ShowLevelOnly` setting. The setting only affects other modes.

**Pros:**
- No config change required
- "Just works" for the user

**Cons:**
- Behavior diverges silently from other modes
- User may wonder why their season-level items aren't being sunset
- Inconsistent — `ShowLevelOnly=false` means different things depending on mode

## Open Questions

1. **Should this apply to all modes, not just sunset?** The partial-show problem exists for approval and auto-delete too. Is sunset special enough to warrant mode-specific validation, or should we make `ShowLevelOnly=true` the default/recommendation for all deletion modes?

2. **What about shows where only some seasons are stale?** A show with 10 seasons where only seasons 1-3 are old and unwatched — should the entire show be queued, or should we skip it because some seasons are still active? This relates to the show-level aggregation question (queue the show when >N% of seasons are candidates).

3. **Should the UI warn proactively?** When a user has sunset mode enabled and then disables `ShowLevelOnly` on a Sonarr integration, should the settings page show a warning? Or should the engine silently ignore seasons?

4. **What about mixed disk groups?** A disk group with both Radarr and Sonarr integrations — the Radarr items (movies) don't have a show/season concept. The validation should only check Sonarr integrations, not Radarr.

## Implementation Steps (When Ready)

1. **Validation** — Add check in disk group mode change handler and/or `evaluateSunsetMode`
2. **Error message** — Clear, actionable error explaining what to change and why
3. **Frontend warning** — When selecting sunset mode in `RuleDiskThresholds.vue`, show a warning banner if linked Sonarr integrations don't have `ShowLevelOnly` enabled, with a direct link to integration settings
4. **Tests** — Validation tests for the mode change rejection path

## Key Files

- `backend/internal/db/models.go` — `IntegrationConfig.ShowLevelOnly`
- `backend/internal/poller/fetch.go:92-102` — Season filtering logic
- `backend/internal/poller/evaluate.go` — `evaluateSunsetMode`
- `backend/routes/disk_groups.go` — Mode change handler
- `frontend/app/components/rules/RuleDiskThresholds.vue` — Mode selector UI
