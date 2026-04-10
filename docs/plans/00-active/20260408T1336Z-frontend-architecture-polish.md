# Frontend Architecture Polish

**Status:** Planned
**Priority:** Medium (code quality/maintainability)
**Estimated Effort:** L (3-4 days)
**Origin:** Brittleness & Rearchitecture Audit, Category 7 (Findings 7.1-7.5)

---

## Summary

Decompose the two largest page components (Dashboard 1189 lines, Help 1202 lines) into focused sub-components, fix hardcoded English strings that bypass the i18n system, surface silent errors to users instead of swallowing them in `console.warn`, and add HMR cleanup for the SSE handler to prevent duplicate handler accumulation during development.

---

## Current State

### Dashboard Page — `pages/index.vue` (1189 lines)

The dashboard is the single largest Vue component. Its 361-line template contains a monolithic Engine Activity card (277 lines of template, lines 53-329) that renders status banners, sparkline charts, mini sparkline grids, and stats rows all inline. The 828-line script section contains a 90-line event icon switch, a 53-line event icon class switch, 175 lines of SSE handler registration, 178 lines of ECharts option builders, and scattered API fetch functions.

**Template structure:**

| Section | Lines | Candidate |
|---------|-------|-----------|
| Pull-to-refresh indicator | 3-8 | Already extracted (`<PullToRefreshIndicator>`) |
| Page header with date range selector | 10-42 | Inline — small, keep as-is |
| Integration error banner | 44-45 | Already extracted (`<IntegrationErrorBanner>`) |
| Empty state | 47-51 | Already extracted (`<DashboardEmptyState>`) |
| Engine Activity card | 53-329 | **Extract** — 277 lines of template |
| Deletion/Snoozed/Sunset cards | 331-338 | Already extracted |
| Approval queue card | 341 | Already extracted |
| Disk group grid | 343-354 | Already extracted (`<DiskGroupSection>`) |

**Script sections to extract:**

| Section | Lines | Candidate |
|---------|-------|-----------|
| `eventIcon()` switch (39 cases) | 516-606 | Move to `utils/eventIcons.ts` |
| `eventIconClass()` switch (39 cases) | 609-662 | Move to `utils/eventIcons.ts` |
| Sparkline data processing | 919-972 | Move into Engine Activity component |
| ECharts option builders | 977-1155 | Move into Engine Activity component |
| Date range options (hardcoded English) | 462-469 | Fix with `$t()` keys |

### Help Page — `pages/help.vue` (1202 lines)

The help page is 1061 lines of template containing 12 `<details>` accordion sections plus an About section. Each section follows an identical pattern: a `<details>` wrapper with a styled `<summary>` and content body. The repetition is the main decomposition opportunity.

**Template sections:**

| Section | Lines | Size | i18n Status |
|---------|-------|------|-------------|
| Page header | 3-11 | 9 | OK |
| Announcements archive | 14-84 | 71 | OK |
| How Scoring Works | 86-116 | 31 | OK |
| Understanding the Sliders | 118-158 | 41 | OK |
| Data Sources | 160-224 | 65 | OK |
| Reading a Score Detail | 226-289 | 64 | OK |
| Threshold & Target | 291-321 | 31 | OK |
| Custom Rules | 323-371 | 49 | OK |
| Tiebreaker | 373-422 | 50 | OK |
| Reading the Audit Log | 424-466 | 43 | OK |
| Execution Modes | 468-534 | 67 | OK |
| FAQ | 536-566 | 31 | OK |
| **Collection Deletion** | **568-726** | **158** | **Hardcoded English** |
| Show-Level Evaluation | 728-794 | 67 | OK |
| About Capacitarr | 796-1058 | 262 | Partial (tech stack/credits hardcoded) |

### Hardcoded English (i18n Gaps)

**`pages/help.vue` — Collection Deletion section (lines 568-726):**
- 158 lines of raw English prose: headings, danger callout, "What it does", "Where collections come from" (Radarr/Plex/Jellyfin/Emby descriptions), "Multiple sources", "Things to watch out for", "Safety features", "Before you enable this", "How to enable"
- None use `$t()` — all other help sections do

**`pages/help.vue` — About section (lines 796-1058):**
- `techStack` array (lines 1172-1192) and `credits` array (lines 1194-1201) have hardcoded English descriptions

**`components/settings/SettingsIntegrations.vue`:**
- Line 169: `>Collection Deletion</span`
- Line 303: `>Collection Deletion</UiLabel`
- Line 322: `Learn more about collection deletion`
- Line 371: `{{ editingIntegration ? 'Save' : 'Add' }}` (Save/Add buttons)
- Line 389: `Enable Collection Deletion?` (dialog title)
- Lines 391-399: Dialog description text
- Line 406: `Learn more about collection deletion and safety features`
- Line 411: `Cancel`
- Line 413: `Yes, enable collection deletion`

**`pages/index.vue` — Date range selector (lines 462-469):**
- Labels: "Last Hour", "Last 6h", "Last 24h", "Last 7 Days", "Last 30 Days", "All Time"

### Silent Error Handling (28 locations)

All of the following catch errors and log to `console.warn` without showing any user-visible feedback:

**Pages:**

| File | Line | Context |
|------|------|---------|
| `pages/index.vue` | 825 | Integration refresh failed |
| `pages/index.vue` | 842 | Settings refresh failed |
| `pages/index.vue` | 913 | Dashboard data fetch failed |
| `pages/index.vue` | 1177 | Engine history fetch failed |
| `pages/index.vue` | 1186 | Recent activity fetch failed |
| `pages/rules.vue` | 75 | Disk groups fetch failed |
| `pages/rules.vue` | 95 | Factor weights fetch failed |
| `pages/rules.vue` | 143 | Integrations fetch failed |
| `pages/rules.vue` | 151 | Rules fetch failed |
| `pages/library.vue` | 280 | Integrations fetch failed |

**Composables:**

| File | Line | Context |
|------|------|---------|
| `composables/useEngineControl.ts` | 217 | Stats fetch failed |
| `composables/useApprovalQueue.ts` | 225 | Queue fetch failed |
| `composables/useSunsetQueue.ts` | 32 | Sunset items fetch failed |
| `composables/useVersion.ts` | 36 | API version fetch failed |
| `composables/useVersion.ts` | 54 | Update check failed |
| `composables/useVersion.ts` | 73 | Manual check failed |
| `composables/usePreview.ts` | 42 | Preview fetch failed |
| `composables/useEventStream.ts` | 214 | Handler error for event type |
| `composables/useConnectionHealth.ts` | 95 | Health poll failed |

**Components:**

| File | Line | Context |
|------|------|---------|
| `components/ScoreBreakdown.vue` | 169 | Score detail parse failed |
| `components/ScoreDetailModal.vue` | 271 | Factor parse failed |
| `components/AuditLogPanel.vue` | 444 | Audit log fetch failed |
| `components/RuleBuilder.vue` | 547 | Rule edit initialization failed |
| `components/settings/SettingsAdvanced.vue` | 681 | Preferences fetch failed |
| `components/settings/SettingsGeneral.vue` | 629 | Preferences fetch failed |
| `components/settings/SettingsSecurity.vue` | 329 | API key fetch failed |

**Intentionally silent (should remain as-is):**

| File | Line | Reason |
|------|------|--------|
| `composables/useConnectionHealth.ts` | 95 | Background polling — toast on every failure would spam |
| `utils/plexOAuth.ts` | 307 | Popup cleanup — benign failure |
| `app.vue` | 44 | Announcement fetch — non-critical |
| `app.vue` | 108 | Migration check — non-critical |

### HMR SSE Handler Accumulation (Finding 7.5)

`composables/useEventStream.ts` (238 lines) uses module-level state (`_eventSource`, `_handlers` Map, `_registeredTypes` Set) that persists across HMR module replacements. The `import.meta.hot` API is not used anywhere in the SSE composable. During development, HMR replaces the composable module but the old `EventSource` connection and handler map survive, causing duplicate handler registrations.

Singleton composables (`useEngineControl`, `useDeletionQueue`, `useSnoozedItems`, `useSunsetQueue`, `useApprovalQueue`) use module-level `_sseRegistered` boolean flags that also survive HMR, preventing re-registration of handlers that reference stale closures.

---

## Implementation Steps

### Phase 1: Dashboard Decomposition

1. **Create `components/dashboard/` subdirectory** following the existing pattern (`components/rules/`, `components/settings/`).

2. **Extract `EngineActivityCard.vue`** from `pages/index.vue` lines 53-329 (template) and the supporting script: sparkline data processing (lines 919-972), ECharts option builders (lines 977-1155), countdown timer (lines 683-713), and engine stats computed (lines 664-678). The new component receives props for dashboard data and emits events for user interactions (run-now, date range change). Move the SSE handlers for engine-related events into the new component using the existing `sseOn()` pattern with scope cleanup.

3. **Extract `utils/eventIcons.ts`** containing `eventIcon()` (lines 516-606) and `eventIconClass()` (lines 609-662). These are pure functions mapping event type strings to Lucide icon components and CSS classes. Import them in `EngineActivityCard.vue` and any other consumers.

4. **Verify dashboard page is under 500 lines** after extraction. The remaining `index.vue` should be ~400-500 lines: imports, composable setup, SSE subscriptions for non-engine events, `fetchDashboardData()`, and the simplified template delegating to child components.

5. **Tests:** Verify the dashboard renders correctly after decomposition. Run `make ci`.

### Phase 2: Help Page Decomposition

6. **Create `components/help/` subdirectory.**

7. **Extract `HelpSection.vue`** — a reusable component wrapping the `<details>` accordion pattern. Props: `title` (string), `icon` (optional Lucide component), `defaultOpen` (boolean). Slot: section content. This replaces the repeated `<details><summary>...</summary>...</details>` pattern used 12 times.

8. **Extract `HelpAbout.vue`** from lines 796-1058 (262 lines). This is the "About Capacitarr" section with project info, tech stack table, credits, and community links. It is self-contained with no dependencies on help page state.

9. **Extract `HelpCollectionDeletion.vue`** from lines 568-726 (158 lines). This isolates the section that needs i18n work (Phase 3) into its own component, making the i18n migration easier to review.

10. **Refactor remaining help sections** to use `<HelpSection>` wrapper. Each section becomes a `<HelpSection :title="$t('help.scoring.title')">` with the content as slot children. This significantly reduces template repetition.

11. **Verify help page is under 400 lines** after extraction. The remaining `help.vue` should be ~300-400 lines: imports, data arrays, and the template composing `<HelpSection>` instances.

12. **Tests:** Verify the help page renders correctly. Run `make ci`.

### Phase 3: i18n Gap Closure

13. **Add i18n keys for the Collection Deletion section.** Add keys under `help.collectionDeletion.*` in `en.json` for all prose content in `HelpCollectionDeletion.vue`. Replace all hardcoded English strings with `$t()` calls.

14. **Add i18n keys for `SettingsIntegrations.vue` Collection Deletion strings.** Add keys under `settings.integrations.collectionDeletion.*` in `en.json` for: section labels (lines 169, 303), learn-more links (lines 322, 406), dialog title (line 389), dialog description (lines 391-399), dialog buttons (lines 411, 413), and the Save/Add button (line 371).

15. **Add i18n keys for Dashboard date range options.** Add keys under `dashboard.dateRange.*` in `en.json` for: "Last Hour", "Last 6h", "Last 24h", "Last 7 Days", "Last 30 Days", "All Time" (lines 462-469).

16. **Add i18n keys for Help About section** where hardcoded English exists in `techStack` and `credits` arrays.

17. **Verify i18n completeness:** Run the project's i18n lint/check (if available) or manually verify that no raw English strings remain in the modified components. Search for `>` followed by English words in templates of the modified files.

18. **Tests:** Run `make ci`.

### Phase 4: Silent Error Surfacing

19. **Define the error surfacing strategy.** Not all errors should show toasts — some are better handled with inline error states. Categorize:

    | Category | Treatment | Examples |
    |----------|-----------|---------|
    | **Page data fetch** | Inline error state + retry button | Dashboard data, rules fetch, library fetch |
    | **Background refresh** | Subtle toast (info level) on repeated failure only | SSE-triggered refreshes, integration refresh |
    | **User-initiated action** | Error toast | Manual check-now, rule edit init |
    | **Parse/display** | Inline fallback + console.warn (keep silent) | Score breakdown parse, factor parse |
    | **Background polling** | Keep silent (existing behavior is correct) | Connection health, version check |

20. **Surface page data fetch errors** in `pages/index.vue`, `pages/rules.vue`, and `pages/library.vue`. Replace `console.warn` with `toast.error($t('errors.fetchFailed'))` for initial page load failures. For SSE-triggered refreshes (lines 825, 842), keep silent on individual failures but surface if 3+ consecutive refreshes fail.

21. **Surface composable errors** in `useEngineControl.ts`, `useApprovalQueue.ts`, `useSunsetQueue.ts`, and `usePreview.ts`. Add `toast.error()` for user-visible fetch failures. For `useVersion.ts`, surface only the manual `checkNow` failure (line 73); keep automatic checks silent.

22. **Surface component errors** in `AuditLogPanel.vue` (line 444), `RuleBuilder.vue` (line 547), `SettingsAdvanced.vue` (line 681), `SettingsGeneral.vue` (line 629), and `SettingsSecurity.vue` (line 329). These are all user-initiated navigations where the user expects content — a silent failure leaves them staring at a blank section.

23. **Keep intentionally silent sites unchanged:** `useConnectionHealth.ts:95`, `plexOAuth.ts:307`, `app.vue:44`, `app.vue:108`, `ScoreBreakdown.vue:169`, `ScoreDetailModal.vue:271`. Document the rationale with a code comment at each site (e.g., `// Intentionally silent — background polling, toast would spam`).

24. **Tests:** Run `make ci`.

### Phase 5: HMR SSE Cleanup

25. **Add `import.meta.hot` cleanup to `useEventStream.ts`.** At module scope, add:
    ```ts
    if (import.meta.hot) {
      import.meta.hot.dispose(() => {
        disconnectSSE()
        _handlers.clear()
        _registeredTypes.clear()
      })
    }
    ```
    This ensures that when the module is replaced during HMR, the old `EventSource` is closed and handler maps are cleared. The new module instance will re-establish the connection when components re-mount.

26. **Add `import.meta.hot` cleanup to singleton composables** (`useEngineControl.ts`, `useDeletionQueue.ts`, `useSnoozedItems.ts`, `useSunsetQueue.ts`, `useApprovalQueue.ts`). Reset their `_sseRegistered` flags in the dispose callback so handlers re-register with fresh closures after HMR.

27. **Manual verification:** Run the dev server, trigger an HMR update, and verify in browser devtools that only one `EventSource` connection exists and events are not handled multiple times.

28. **Tests:** Run `make ci`.

---

## Out of Scope

- **Frontend test coverage expansion** (Audit Finding 8.1) — tracked separately, incremental effort
- **vue-sonner migration** — already completed in the audit's Phase 4
- **Dead UI component removal** — already completed in the audit's Phase 4
- **Radix icons replacement** — already completed in the audit's Phase 4

## Risk Assessment

- **Low regression risk** — all changes are refactoring (component extraction, string extraction) with no logic changes
- **Phase 1-2** (decomposition) are the highest effort but purely structural — no behavior changes
- **Phase 3** (i18n) affects all 22 locale files — non-English locales will show English keys until translated, which is the existing pattern for new keys
- **Phase 4** (error surfacing) changes user-visible behavior — some previously silent failures will now show toasts; err on the side of subtle feedback
- **Phase 5** (HMR) is dev-only — zero production impact
