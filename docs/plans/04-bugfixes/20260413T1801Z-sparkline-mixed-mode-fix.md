# Fix Engine Activity Sparkline for Mixed Per-Disk-Group Modes

**Status:** ✅ Complete
**Branch:** `fix/sparkline-mixed-mode`
**Scope:** Frontend only (`frontend/app/pages/index.vue`)
**Supersedes:** Partially revises the mode-aware sparkline design from `docs/plans/03-ui-ux/20260323T1948Z-mode-aware-sparkline.md`

## Problem

The engine activity sparkline uses a single `isDryRunMode` boolean (derived from `allDryRun`) to decide which series to display:

- **All dry-run** → amber "Would Delete" series from `queued` field
- **Any non-dry-run** → red "Deleted" series from `deleted` field

Since v3.0, execution mode is per-disk-group. When disk groups run in different modes (e.g., group A in `auto`, group B in `dry-run`), both `queued` and `deleted` carry meaningful data in the same engine run. The current either/or toggle causes:

1. **Under-reporting:** In mixed mode, `allDryRun` is `false`, so only the red "Deleted" series renders. The `queued` contributions from dry-run groups are invisible.
2. **Wrong ghost series:** The ghost (dashed) series shows the "inactive" mode's data, but with mixed modes there is no single inactive mode — both are active simultaneously.
3. **Misleading tooltip:** The "dry-run — no deletions" hint renders only when `allDryRun` is true, which is never the case in mixed mode even though some groups are in dry-run.

## Solution

Always render both `queued` and `deleted` as independent, permanent series alongside the existing `candidates` series. Remove the `isDryRunMode` toggle from the sparkline entirely. The data self-selects: when a field is zero for all points in the range, its line is flat at the baseline and visually disappears.

This is a frontend-only change. The backend already stores both fields correctly per engine run.

## Design

### Series (always rendered)

| Series | Source field | Color | Label |
|--------|-------------|-------|-------|
| Candidates | `candidates` | Primary (blue) | "Candidates" |
| Would Delete | `queued` | Amber (`chart3Color`) | "Would Delete" |
| Deleted | `deleted` | Destructive (red) | "Deleted" |

### Legend

All three legend dots render unconditionally. When a series has no non-zero data in the current range, its legend dot is dimmed (`opacity-40`) to signal "no data" without hiding the label.

### Tooltip

- All three series shown (skip any with value 0 to reduce noise)
- Mode-aware hint replaced with a per-point contextual note: if `queued > 0 && deleted === 0` → "(dry-run)", if `deleted > 0 && queued === 0` → "(live)", if both non-zero → "(mixed modes)"
- Ghost series entries no longer appear (ghost series removed)

### Real-time SSE patch

`handleDeletionProgressSparkline()` currently patches either `queued` or `deleted` based on `isDryRunMode`. The `deletion_progress` SSE event only fires during actual deletions (auto/approval groups), so the patch field should always be `deleted`. The `queued` count for dry-run groups is finalized by `UpdateRunStats()` at the end of the engine run and arrives via the `engine_run_complete` SSE event, so it does not need real-time patching.

---

## Steps

### Step 1: Remove ghost series from `sparklineEChartsOption`

**File:** `frontend/app/pages/index.vue` (lines 1072–1085)

Remove the ghost series block entirely. With both `queued` and `deleted` always visible, there is no concept of an "inactive mode" to ghost.

**Remove:**
```ts
// Ghost series: inactive mode's data (faint dashed, no interaction)
if (ghostSeriesData.length > 0 && ghostSeriesData.some((d) => d.y > 0)) {
  series.push({
    name: ghostName + ' (historical)',
    // ...
  });
}
```

**Also remove** the supporting variables at the top of the computed:
```ts
const ghostSeriesData = isDryRun ? deletedSeries.value : queuedSeries.value;
const ghostName = isDryRun ? t('dashboard.deleted') : t('dashboard.wouldDelete');
const ghostColor = isDryRun ? destructiveColor.value : chart3Color.value;
```

---

### Step 2: Replace toggled second series with two permanent series

**File:** `frontend/app/pages/index.vue` (lines 1011–1070)

Replace the `isDryRun`-branched second series with two unconditional series blocks.

**Remove** the toggled variables:
```ts
const isDryRun = isDryRunMode.value;
const secondSeriesData = isDryRun ? queuedSeries.value : deletedSeries.value;
const secondName = isDryRun ? t('dashboard.wouldDelete') : t('dashboard.deleted');
const secondColor = isDryRun ? chart3Color.value : destructiveColor.value;
```

**Replace** with two permanent series pushes:

```ts
// "Would Delete" series (amber) — always rendered from queued field
if (queuedSeries.value.length > 0) {
  const qData = queuedSeries.value;
  series.push({
    name: t('dashboard.wouldDelete'),
    type: 'line',
    smooth: true,
    symbol: sparseSymbol(qData.length),
    symbolSize: sparseSymbolSize(qData.length),
    itemStyle: { color: chart3Color.value },
    lineStyle: glowLineStyle(chart3Color.value),
    areaStyle: gradientArea(chart3Color.value),
    emphasis: emphasisConfig(),
    data: qData.map((d) => [d.x, d.y]),
  });
}

// "Deleted" series (red) — always rendered from deleted field
if (deletedSeries.value.length > 0) {
  const dData = deletedSeries.value;
  const lastPoint = dData[dData.length - 1];
  series.push({
    name: t('dashboard.deleted'),
    type: 'line',
    smooth: true,
    symbol: sparseSymbol(dData.length),
    symbolSize: sparseSymbolSize(dData.length),
    itemStyle: { color: destructiveColor.value },
    lineStyle: glowLineStyle(destructiveColor.value),
    areaStyle: gradientArea(destructiveColor.value),
    emphasis: emphasisConfig(),
    data: dData.map((d) => [d.x, d.y]),
    // Animated pulse on rightmost point while engine is running
    markPoint:
      engineIsRunning.value && lastPoint
        ? {
            symbol: 'circle',
            symbolSize: 8,
            data: [{ coord: [lastPoint.x, lastPoint.y] }],
            itemStyle: { color: destructiveColor.value },
            animation: true,
            animationDuration: 1200,
            animationEasingUpdate: 'sinusoidalInOut',
          }
        : undefined,
  });
}
```

The `markPoint` pulse moves to the "Deleted" series because real-time deletion progress only applies to actual deletions.

---

### Step 3: Update legend in template

**File:** `frontend/app/pages/index.vue` (lines 138–153)

Replace the toggled legend with three permanent legend entries:

```html
<span class="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
  <span class="w-2 h-2 rounded-full bg-primary" />
  {{ $t('dashboard.candidates') }}
</span>
<span
  class="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
  :class="{ 'opacity-40': !hasQueuedData }"
>
  <span class="w-2 h-2 rounded-full bg-amber-500" />
  {{ $t('dashboard.wouldDelete') }}
</span>
<span
  class="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
  :class="{ 'opacity-40': !hasDeletedData }"
>
  <span class="w-2 h-2 rounded-full bg-destructive" />
  {{ $t('dashboard.deleted') }}
</span>
```

Add two helper computeds:

```ts
const hasQueuedData = computed(() => queuedSeries.value.some((d) => d.y > 0));
const hasDeletedData = computed(() => deletedSeries.value.some((d) => d.y > 0));
```

---

### Step 4: Update tooltip formatter

**File:** `frontend/app/pages/index.vue` (lines 1115–1130)

Replace the `isDryRun` footer hint with a per-point contextual note:

```ts
formatter: (
  params: Array<{ seriesName: string; value: [number, number]; marker: string }>,
) => {
  if (!params.length) return '';
  const ts = new Date(params[0]!.value[0]).toLocaleString();
  let html = `<div style="font-weight:600">${ts}</div>`;
  let hasQueued = false;
  let hasDeleted = false;
  for (const p of params) {
    if (p.value[1] === 0) continue; // skip zero-value series
    html += `<div>${p.marker} ${p.seriesName}: <b>${Math.round(p.value[1])}</b></div>`;
    if (p.seriesName === t('dashboard.wouldDelete')) hasQueued = true;
    if (p.seriesName === t('dashboard.deleted')) hasDeleted = true;
  }
  if (hasQueued && hasDeleted) {
    html += `<div style="opacity:0.6;font-size:11px;margin-top:2px">mixed modes</div>`;
  } else if (hasQueued) {
    html += `<div style="opacity:0.6;font-size:11px;margin-top:2px">dry-run — no deletions</div>`;
  }
  return html;
},
```

---

### Step 5: Fix `handleDeletionProgressSparkline` SSE handler

**File:** `frontend/app/pages/index.vue` (lines 837–844)

Remove the `isDryRunMode` branch. Always patch the `deleted` field:

```ts
function handleDeletionProgressSparkline(data: unknown) {
  const event = data as DeletionProgress;
  const history = engineHistoryData.value;
  const last = history.length > 0 ? history[history.length - 1] : undefined;
  if (last) {
    engineHistoryData.value = [
      ...history.slice(0, -1),
      { ...last, deleted: event.succeeded },
    ];
  }
}
```

---

### Step 6: Clean up unused references

**File:** `frontend/app/pages/index.vue`

- The `isDryRunMode` computed (line 1007) is no longer used by the sparkline. Check if it is referenced elsewhere in the file. If not, remove it. If it is used elsewhere (e.g., for a different UI element), leave it but add a comment noting it is no longer sparkline-related.
- The `allDryRun` computed (lines 500–505) may still be used by other dashboard elements (deletion queue card, etc.). Do not remove it.

---

### Step 7: Run `make ci` and verify

Run `make ci` to ensure lint, tests, and security checks pass.

Build and run the container with `docker compose up --build`. Verify:

1. **All dry-run:** Amber "Would Delete" line visible with data, red "Deleted" line flat, "Deleted" legend dimmed
2. **All auto:** Red "Deleted" line visible with data, amber "Would Delete" line flat, "Would Delete" legend dimmed
3. **Mixed modes:** Both amber and red lines visible with data, both legends at full opacity
4. **Tooltip:** Shows correct contextual hint per hover point
5. **Real-time SSE:** During an engine run with auto groups, the "Deleted" series patches in real-time; "Would Delete" updates on run completion
6. **Sparse data (≤ 3 points):** Symbols visible on both series
7. **Date range switching:** All ranges (1h, 6h, 24h, 7d, 30d, all) render correctly

---

## Files Changed

| File | Step | Change |
|------|------|--------|
| `frontend/app/pages/index.vue` | 1–6 | Remove ghost series, add permanent queued + deleted series, update legend, fix tooltip, fix SSE handler, clean up |

## Backend Changes

None required. `EngineRunStats` already stores both `queued` and `deleted` per engine run.
