# Release Archaeology (CLI)

| Field | Value |
|-------|-------|
| **Status** | Approved (autonomous) |
| **Date** | 2026-07-13 |
| **Scope** | CLI-only: show release snapshot; diff two releases |

## Goal

```bash
launchpad releases show 3
launchpad diff --from-release 2 --to-release 3
```

## Design

- `releases show N` — print JSON of release N from list (full snapshot fields already on list API)
- `diff --from-release A --to-release B` — treat B’s snapshot as “pending fold” vs A as baseline using existing formatDiff helpers (image, config, scale)

Optional later: `--env` cross-env (not this PR).
