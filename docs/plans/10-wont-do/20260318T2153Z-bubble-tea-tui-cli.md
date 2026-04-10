# Bubble Tea TUI/CLI Frontend

> **Status:** ❌ Will not implement
> **Created:** 2026-03-18
> **Closed:** 2026-04-08
> **Original work:** 21 commits implementing a full terminal UI and CLI for Capacitarr (stale `feature/tui-cli` branch, recoverable via SHA `57b9d63`)

## Summary

A terminal-based frontend for Capacitarr using [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Go TUI framework) and [Glamour](https://github.com/charmbracelet/glamour) (markdown rendering). Provides both an interactive TUI and non-interactive CLI commands for headless/SSH management.

## What Was Built (on the stale branch)

The `feature/tui-cli` branch contained a near-complete implementation:

| Phase | Feature | Status |
|-------|---------|--------|
| 1 | Foundation — project scaffold, DataProvider interface, SSE client | ✅ Complete |
| 2 | App shell — Bubble Tea app with view switching, keyboard navigation | ✅ Complete |
| 3 | SSE client — real-time event stream consumption in terminal | ✅ Complete |
| 4 | Dashboard view — disk groups, engine status, activity feed | ✅ Complete |
| 5 | Approval queue view — list, approve, reject, snooze | ✅ Complete |
| 6 | Audit log view — history with filtering | ✅ Complete |
| 7 | Charts — ntcharts engine history sparklines | ✅ Complete |
| 8 | Rules view — rule listing and management | ✅ Complete |
| 9 | Settings view — preferences display and editing | ✅ Complete |
| 10 | Integrations view — integration listing and status | ✅ Complete |
| 11 | Help view — Glamour-rendered markdown help | ✅ Complete |
| 12 | Non-interactive CLI — `capacitarr status`, `capacitarr approve <id>`, etc. | ✅ Complete |
| 13 | Build pipeline integration — TUI binary included in release | ✅ Complete |

## Why It Will Not Be Implemented

The branch was based on an older version of `main` (before queue management, deletion queue, preview service, and many other features). The API surface changed significantly, making the TUI incompatible with the current backend. The cost of reimplementation against the current or 2.0 API does not justify the benefit — the web UI covers all use cases adequately.

## Architecture

- **DataProvider interface** — abstracts API calls so the TUI can work with both live servers and mock data for testing
- **SSE client** — consumes the same `/api/v1/events` stream as the web frontend for real-time updates
- **View switching** — tab-based navigation between Dashboard, Approval Queue, Audit Log, Rules, Settings, Integrations, Help
- **Non-interactive CLI** — `capacitarr status`, `capacitarr approve <id>`, `capacitarr reject <id>`, `capacitarr run` for scripting and cron jobs

## Dependencies

- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — TUI styling
- `github.com/charmbracelet/glamour` — Markdown rendering
- `github.com/NimbleMarkets/ntcharts` — Terminal charts (sparklines, history)

## Historical Notes

The TUI was never part of the 2.0 plan. The non-interactive CLI commands (`capacitarr status`, `capacitarr approve`, `capacitarr run`) were independently valuable as thin API wrappers, but are also not planned.

The stale branch commits were recoverable via SHA `57b9d63`, but may have been garbage collected by now (branch deleted prior to 2026-03-18).
