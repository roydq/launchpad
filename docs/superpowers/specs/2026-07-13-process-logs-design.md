# Process Logs (Control Plane + CLI)

| Field | Value |
|-------|-------|
| **Status** | Approved (autonomous) |
| **Date** | 2026-07-13 |
| **Scope** | Stream/read process logs via API and CLI using existing Target.Logs |

## Goal

```bash
launchpad logs          # process web, current env
launchpad logs worker   # named process
```

## Approach (recommended)

Wire `target.Registry` into the API process (same registration as worker: stub always, k8s when available). Service resolves project/env/service and calls `Target.Logs`.

| Piece | Design |
|-------|--------|
| API | `GET /v1/projects/{project}/logs?process=web` — `text/plain`; env via `X-Launchpad-Environment` |
| Service | `RuntimeService.Logs` |
| CLI | `launchpad logs [process]` default `web` |
| Follow | Out of scope for v1 (no `--follow` long poll) |

## Non-goals

Multi-pod selection UI, log aggregation, SSE.

## Tests

RuntimeService with stub registry returns known stub body.
