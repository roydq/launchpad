# Deploy Wait / Follow (CLI)

| Field | Value |
|-------|-------|
| **Status** | Approved (lightweight) |
| **Date** | 2026-07-11 |
| **Scope** | CLI polls job until terminal after deploy / `--now` |
| **Integration** | Merges to `feat/dx-stack` |

## Goal

```bash
launchpad deploy --image app:v1 --wait
# prints progress, exits 0 on job succeeded, non-zero on failed/dead/timeout
```

## Design

- Flags on `deploy` and mutation `--now` paths: `--wait`, `--timeout` (default 5m)
- After queue, poll `GET /v1/jobs/{id}` every 500ms
- Terminal job statuses: `succeeded` (ok), `failed`/`dead` (error)
- Print status lines: `job status=running …`
- No SSE/server changes; no logs streaming in this slice

## Non-goals

- Target log streaming
- Deployment resource API beyond job poll
- Defaulting all deploys to wait (opt-in flag)
