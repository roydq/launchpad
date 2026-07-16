# Server-side pending / diff preview

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve) |
| **Date** | 2026-07-15 |
| **Scope** | Structured preview of pending changeset vs last release (and release↔release) via API |

---

## Goal

Agents and CI should not reimplement CLI fold/diff. One control-plane call returns **what will change** if you deploy now (or between two releases).

```bash
# Pending vs last deploy in ambient env
GET /v1/projects/{project}/preview
# Release archaeology
GET /v1/projects/{project}/preview?from_release=1&to_release=3
```

CLI `launchpad diff` may call the API for pending mode (text still formatted client-side from structured diff, or use `summary` field).

---

## Approach

Move fold + effective-diff logic into `internal/service` (same rules as CLI). API returns JSON. No new entities.

## Out of scope

Env↔env live config diff, dry-run deploy, multi-service.

## Response sketch

```json
{
  "mode": "pending",
  "environment": "dev",
  "baseline_version": 2,
  "has_pending": true,
  "matches_baseline": false,
  "pending": {
    "image": "app:v2",
    "config": { "PORT": "8080", "DEBUG": null },
    "scales": { "web": 3 }
  },
  "diff": {
    "image": { "from": "app:v1", "to": "app:v2" },
    "config": [
      { "op": "change", "key": "PORT", "from": "3000", "to": "8080" },
      { "op": "remove", "key": "OLD", "from": "x" },
      { "op": "add", "key": "NEW", "to": "y" }
    ],
    "scale": [
      { "process": "web", "from": 1, "to": 3 }
    ]
  },
  "summary": "## Image\n..."
}
```

`config` null value = staged unset. Empty pending → `has_pending=false`, empty `diff`.
