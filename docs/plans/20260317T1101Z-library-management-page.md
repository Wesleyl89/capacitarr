# Library Management Page

**Status:** 📋 Planned
**Branch:** `feature/library-management`
**Created:** 2026-03-17
**Depends on:** `feature/rule-filter-force-delete` (force-delete backend)

## Overview

A dedicated Library Management page that shows the full media library across all integrations with flat (ungrouped) rows. Each item — movie, season, artist, book — is independently selectable for force-delete. This replaces the selection-mode approach that was originally planned for the Deletion Priority view.

## Motivation

The Deletion Priority view on the Scoring Engine page is a read-only scoring preview. Adding destructive actions (force-delete) there blurs the line between preview and action. A dedicated Library Management page provides:

- Clear separation: Scoring Engine = configure & preview, Library Management = take action
- Full library visibility: manage any item, not just engine-flagged ones
- Flat season rows: each season is independently selectable without grouping complexity
- Extensible: future actions (tag, protect, exclude) can live here

## Design

### Page Layout

New page at `/library` with a nav link. Contains:
1. Search bar + filters (by integration, media type, tags)
2. View mode toggle (table/poster)
3. Flat item list (no show→season grouping)
4. Selection mode with floating action bar
5. Force-delete confirmation dialog

### Data Source

Reuses the existing `/api/v1/preview` endpoint which fetches, enriches, and scores all media items. The Library Management page displays the same data but:
- **No grouping**: Each season is its own row/card (flat list)
- **Sorted by title** by default (not by score)
- **All items shown**: No deletion line or threshold context

### Table Mode (Flat Rows)

```
☐  The Big Door Prize - Season 2    Season  Sonarr  19.7 GB  Score: 3.75
☐  The Big Door Prize - Season 1    Season  Sonarr  18.2 GB  Score: 3.71
☐  Deal or No Deal Island - S2      Season  Sonarr  34.9 GB  Score: 3.70
☐  Serenity                         Movie   Radarr   2.1 GB  Score: 0.82
```

Columns: Checkbox, Title, Type, Integration, Size, Score

### Poster Mode (Flat Cards)

Each season gets its own card with checkbox overlay in selection mode. No popover grouping.

### Selection Mode

Same pattern as Sonarr/Radarr mass editor:
1. Click "Select" button to enter selection mode
2. Checkboxes appear on all items
3. Click items to select/deselect
4. Floating action bar shows count + total size + "Force Delete" button
5. Confirmation dialog lists selected items
6. API call to `POST /api/v1/force-delete`

### Season Granularity

Since the list is flat (no grouping), season-level selection is automatic:
- To delete a whole show: select all its seasons
- To delete one season: select just that season
- Each season has its own `externalId` and can be independently deleted

## Implementation Steps

### Step 1: Create Library Management Page

Create `frontend/app/pages/library.vue` with the page layout, data fetching (reuse preview API), and flat item display.

**Files:**
- `frontend/app/pages/library.vue` — New page component

### Step 2: Add Nav Link

Add "Library" link to the navbar between "Scoring Engine" and "Audit Log".

**Files:**
- `frontend/app/components/Navbar.vue` — Add library nav item

### Step 3: Flat Item Table Component

Create a `LibraryTable.vue` component that displays items as flat rows (no grouping). Includes search, filter by integration/type, sort by title/size/score/type.

**Files:**
- `frontend/app/components/LibraryTable.vue` — New component

### Step 4: Selection Mode + Floating Action Bar

Add selection mode toggle, checkbox column/overlay, floating action bar with count + Force Delete button.

**Files:**
- `frontend/app/components/LibraryTable.vue` — Selection state, checkboxes, floating bar

### Step 5: Force-Delete Confirmation Dialog

Confirmation dialog listing selected items with names and sizes. Calls `POST /api/v1/force-delete`.

**Files:**
- `frontend/app/components/LibraryTable.vue` — Confirmation dialog

### Step 6: i18n Strings

Add English strings for the Library Management page, then propagate to all locales.

**Files:**
- `frontend/app/locales/en.json` — New strings
- `frontend/app/locales/*.json` — Propagate to all locales

### Step 7: Tests + CI

Run `make ci` to verify all changes pass.

## Safety Considerations

- Force-delete respects `DeletionsEnabled` preference (API returns 409 if disabled)
- Force-delete is blocked in dry-run mode (API returns 409)
- Confirmation dialog clearly states items will be deleted on next engine run
- Protected items (always_keep rule) have checkboxes disabled
