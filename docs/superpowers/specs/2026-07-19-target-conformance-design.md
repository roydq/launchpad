# Target conformance suite

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Scope** | Shared conformance tests for `target.Target`; run against stub in unit CI |
| **Queue** | `target-conformance` |

## Goal

One checklist that any Target implementation must pass: Deploy → Status → Logs → Scale → Rollback → Destroy.

```go
conformance.Run(t, stub.New())
```

Kubernetes remains optional (kind e2e); suite is in-process only for stub.

## Approach A (recommended)

`internal/target/conformance` with `Run(t *testing.T, tgt target.Target)` using synthetic domain fixtures. Stub test file calls Run.

## Self-review

Pass — Track B only.
