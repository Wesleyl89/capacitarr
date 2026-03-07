# API Versioning & Stability Guarantees

This document describes Capacitarr's API versioning policy, what constitutes a breaking change, and what API consumers can rely on remaining stable.

## Current API Version

The stable API is **v1**, served at:

```
http://localhost:2187/api/v1/
```

All endpoints documented in the [OpenAPI spec](openapi.yaml) are under this prefix.

## Semantic Versioning

Capacitarr follows [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`). Here's how semver maps to API changes:

| Version Bump | What Changes |
|---|---|
| **PATCH** | Bug fixes to existing endpoint behavior, documentation corrections |
| **MINOR** | New endpoints, new optional query parameters, new fields added to responses, new optional fields in request bodies |
| **MAJOR** | URL prefix changes (`/api/v2/`), field removals, field type changes, endpoint removals, breaking changes to auth behavior |

## What Is a Breaking Change

The following are considered **breaking changes** and will only occur in a major version bump:

- Removing a field from a response body
- Changing a field's type (e.g., `string` → `number`)
- Removing an endpoint
- Changing an endpoint's HTTP method
- Making a previously optional request field required
- Changing authentication requirements (e.g., making a public endpoint require auth)
- Changing the error response format (structure, not message text)

## What Is NOT a Breaking Change

The following are **non-breaking** and may occur in any minor or patch release:

- Adding new fields to response bodies
- Adding new endpoints
- Adding new optional query parameters
- Adding new optional fields to request bodies
- Adding new enum values to existing fields (e.g., new integration types)
- Changing error messages (the text, not the structure)
- Performance improvements

## Stability Guarantees

API consumers can rely on the following remaining stable within `/api/v1/`:

- **No breaking changes** to `/api/v1/` endpoints without a major version bump
- **All three authentication methods** — API key header, Bearer JWT, and Cookie JWT — are stable
- **Response JSON field names** use `camelCase` and will not be renamed
- **HTTP status codes** for success (`200`, `201`) and error (`400`, `401`, `403`, `404`, `409`, `500`) cases are stable
- **Error response format** — `{"error": "message"}` — is stable

## Deprecation Policy

When an endpoint or field needs to be removed:

1. It will be **deprecated first** with a minimum notice period before removal
2. Deprecated endpoints will include a `Deprecation` HTTP header in responses
3. A **`/api/v2/`** version will be introduced before any breaking changes take effect
4. Both `/api/v1/` and `/api/v2/` will **run simultaneously** during a transition period to give consumers time to migrate

## Recommendations for API Consumers

- **Ignore unknown fields** in responses — new fields may be added at any time (forward compatibility)
- **Don't rely on field ordering** in JSON responses
- **Use the OpenAPI spec** as the source of truth for request/response shapes
- **Pin to a specific API version** (`/api/v1/`) — don't use unversioned endpoints

## Breaking Changes in v2.0.0

The 2.0.0 release includes the following breaking changes from the pre-release (1.0.0-rc.x) series:

### Database

- **Fresh database schema.** The 18 incremental migrations have been replaced with a single baseline migration. Existing databases from 1.0.0-rc.x are **not compatible** — users start fresh on upgrade.
- The `audit_logs` table has been split into two purpose-specific tables:
  - `approval_queue` — active items in the approval workflow (state machine: `pending` → `approved`/`rejected`)
  - `audit_log` — permanent, append-only history of deletions and dry-runs

### API Endpoints

| Old Endpoint | New Endpoint | Notes |
|---|---|---|
| `GET /api/v1/audit` | `GET /api/v1/audit-log` | History only (deleted, dry-run, dry-delete) |
| `GET /api/v1/audit/recent` | `GET /api/v1/audit-log/recent` | Dashboard mini-feed (history) |
| `GET /api/v1/audit/grouped` | `GET /api/v1/audit-log/grouped` | Grouped history view |
| `POST /api/v1/audit/:id/approve` | `POST /api/v1/approval-queue/:id/approve` | Approve a queued item |
| `POST /api/v1/audit/:id/reject` | `POST /api/v1/approval-queue/:id/reject` | Reject (snooze) a queued item |
| `POST /api/v1/audit/:id/unsnooze` | `POST /api/v1/approval-queue/:id/unsnooze` | Requeue a snoozed item |
| *(none)* | `GET /api/v1/approval-queue` | List queued items |
| *(none)* | `GET /api/v1/events` | SSE real-time event stream |

### Response Schema Changes

- `AuditLog` → `AuditLogEntry`: removed `snoozedUntil` field, `action` restricted to `deleted`/`dry_run`/`dry_delete`
- New `ApprovalQueueItem` type with `status` field (`pending`/`approved`/`rejected`) instead of overloaded `action`
- Activity events are now streamed via SSE in addition to the REST endpoint

### Architecture

- All business logic moved from route handlers to a service layer
- Event bus replaces direct `LogActivity()` calls — 39 typed event types
- SSE replaces polling for real-time UI updates
- Notification dispatch is event-driven (subscriber) instead of inline calls
