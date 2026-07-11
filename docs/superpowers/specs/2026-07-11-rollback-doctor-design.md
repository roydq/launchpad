# Rollback + Doctor

| Field | Value |
|-------|-------|
| **Status** | Approved (lightweight) |
| **Date** | 2026-07-11 |
| **Integration** | `feat/dx-stack` |

## Rollback

`POST /v1/projects/{project}/rollback` with `{ "version": N, "description": "..." }`  
Header env selects target environment.

Creates **new** release: artifact + process_snapshot from prior version; config re-resolved from live tables for current env; enqueue deploy.

CLI: `launchpad rollback N [-m] [--wait]`

## Doctor

`launchpad doctor` — healthz, token present, list projects, optional project/env resolve.
