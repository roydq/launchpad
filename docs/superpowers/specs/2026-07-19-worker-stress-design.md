# Worker lease / supersede stress tests

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Scope** | Concurrent lease uniqueness + reclaim + supersede stress unit tests |
| **Queue** | `worker-stress` |

## Goal

Prove that under concurrent workers:

1. Each queued job is leased by at most one worker.
2. Expired leases are reclaimed and can be leased again.
3. Supersede of running deployments remains correct under sequential second deploy (existing coverage + concurrent lease of independent jobs).

## Approach

SQLite in-memory with serializable transactions (current store). Enqueue N jobs, spawn M goroutines calling LeaseNext, assert N unique IDs leased, no duplicates.

## Self-review

Pass — Track B only.
