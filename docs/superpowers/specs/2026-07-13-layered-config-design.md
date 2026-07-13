# Layered Config (Phase 2b, shared + service)

| Field | Value |
|-------|-------|
| **Status** | Approved (autonomous) |
| **Date** | 2026-07-13 |
| **Scope** | Shared (project×env) + service (service×env) layers; resolve at release |

## Goal

```bash
launchpad config set --shared LOG_LEVEL=info
launchpad config set PORT=3000
launchpad deploy --image app:v1 --wait
# release.config_resolved has LOG_LEVEL + PORT
```

## Approach

| Layer | Storage | Override order |
|-------|---------|----------------|
| shared | `shared_config_vars (project_id, environment_id, key)` | earlier |
| service | existing `config_vars` | later (wins) |
| workspace / platform | deferred empty | — |

Resolution only at release create / push (invariant preserved).

## Staging

`StageChangeInput` gains optional `layer`: `service` (default) | `shared`.  
Push applies shared then service merges, then `ResolveConfig` for snapshot.

## API

- `GET …/config` → **resolved** map  
- `GET …/config?layer=shared|service` → single layer  
- Stage/push use layer on changes; PATCH config remains service layer (compat)

## CLI

- `config set [--shared] KEY=VAL`  
- `config get` shows resolved; `config get --shared` / `--service` for raw layer  

## Non-goals

Workspace layer, secrets typing, bindings.
