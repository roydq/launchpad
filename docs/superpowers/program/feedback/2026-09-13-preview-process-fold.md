# Persona: preview-process-fold

| Field | Value |
|-------|-------|
| **Date** | 2026-09-13 |
| **Feature** | QUEUE `preview-process-fold` |
| **Script** | S1-equivalent: `process set` then `launchpad diff` (stub API) |

## Environment

Stub API + worker from `.worktrees/feat-preview-process-fold` binaries (`LAUNCHPAD_API_ADDR=127.0.0.1:18086`). CLI `./bin/launchpad`.

## What I did

1. `launchpad projects create persona-preview --target stub --namespace default`
2. `launchpad use persona-preview`
3. `launchpad process set worker --command "run-worker"`
4. `launchpad diff`
5. `launchpad reset` then `launchpad scale worker=3` then `launchpad diff` (scale-on-missing-name check)

## What I saw

After process set, `launchpad diff` printed:

```
## Process
  + worker command="run-worker" quantity=1 expose=none
```

No 400. Intelligible before deploy.

After scale `worker=3` with no worker definition:

```
## Scale
  worker: (none) → 3
```

No invented `## Process` add (matches push 404 on unknown scale name).

## Severity

None P0. Day-one process preview is usable.

## Notes

Did not run full S1 deploy `--wait` in this slice; e2e-stub `TestPreviewPendingProcessSet` plus existing happy-path deploy cover the API/worker. S4 not run (no new error-path CLI).
