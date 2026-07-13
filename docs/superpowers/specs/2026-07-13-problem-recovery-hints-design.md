# Problem+json recovery hints

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve, spike program) |
| **Date** | 2026-07-13 |
| **Scope** | RFC 7807 extension members `code` + `hints` on API errors |

---

## Goal

Agents and humans get structured recovery guidance on API failures without scraping free-text `detail` alone.

```json
{
  "type": "https://launchpad.dev/errors/Conflict",
  "title": "Conflict",
  "status": 409,
  "detail": "conflict: deployment already in progress",
  "code": "deployment_in_progress",
  "hints": [
    {"action": "wait", "message": "A deploy is already running for this service and environment.", "command": "launchpad deploy --wait"}
  ]
}
```

## Approach

Catalog in `problem.HintsFor(err)` matched via `errors.Is` + detail substrings. Wire through `writeError`. Optional apiclient structured error.

## Out of scope

- Server-side diff preview
- Full coded error taxonomy in service layer
- MCP / OpenAPI
