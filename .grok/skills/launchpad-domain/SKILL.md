---
name: launchpad-domain
description: >
  Launchpad domain model and API conventions. Use when changing entities,
  schema, releases, deployments, changesets, environments, services, processes,
  targets, or REST paths. Use when asked about invariants, MVP scope, or how
  Launchpad differs from Heroku. Triggers on "domain model", "entity", "project",
  "environment", "service", "changeset", "release", "deployment", "target interface".
---

# Launchpad Domain

## First step

Read `docs/DOMAIN.md` in full before proposing or implementing domain changes.

For **current shipped vs deferred scope**, use `docs/DOMAIN.md` (reading guide + phased table) and `AGENTS.md` MVP boundaries. The greenfield doc `docs/superpowers/specs/2026-07-04-mvp-core-greenfield-design.md` is a **historical implementation record only** — do not treat its deferred list as current.

## Core hierarchy

```
Workspace → Project → Environment + Service → Process
Release (per Service) → Deployment (Service × Environment)
Changeset (per Project, 0..1 open)
```

**Never use:** `App`, `app_id`, `/v1/apps`, duplicate apps per environment.

## Principles (enforce in every layer)

1. Releases are **immutable** — rollback = new release with old artifact.
2. Config is **resolved at release creation** into `config_resolved` / snapshot fields.
3. **Environments own targets** (`target_type`, `target_config`), not projects.
4. **Changesets stage intent**; push creates release(s) + deployment(s) + job(s).
5. **Composition via refs** (`${{ services.* }}`), not parent/child apps (deferred).
6. Multi-service push requires explicit `parallel` | `atomic` mode (deferred until multi-service).

## MVP cut line

| Shipped (in scope) | Deferred |
|--------------------|----------|
| Multi-env (ambient header + CLI `env *`), promote, env clone | Multi-service, ReleaseSet, coordination modes |
| Layered config (shared + service); secret AES-GCM at rest | Workspace config layer, bindings (`${{ refs }}`) |
| Single primary service; image-only releases | In-cluster builds |
| Rollback, logs, preview, audit, deploy worker | Scale as worker job, SSE/events, cancel |
| Runtime depth: process set/unset/apply, health, immutable cfg, extensions + capabilities | OIDC login, idempotency, Helm |

Do not add deferred tables or API routes without updating the domain spec first.

## Layer mapping

| Concern | Package |
|---------|---------|
| Types + FSM | `internal/domain/` |
| Persistence | `internal/store/` |
| Orchestration | `internal/service/` |
| HTTP | `internal/api/` |
| Async | `internal/jobs/` |
| Runtime backends | `internal/target/` |

Domain types must not import store, api, or target packages.

## Bootstrap invariant

`POST /v1/projects` (or `store.CreateProject`) must atomically create:

1. Project (`primary_service` = project name)
2. Environment `dev` with target config
3. Service (same name as project)
4. Process `web` (quantity=1, expose=http)

## Deployment concurrency

At most one active deployment per `(service_id, environment_id)` where status ∈ `pending`, `deploying`.

## Target interface

```go
type DeployRequest struct {
    Project, Service, Environment, Release
    Processes []Process
    Config    map[string]string // resolved only
}
```

Artifact comes from `Release.ArtifactRef`, not a separate image field.

## When the model needs to grow

1. Update `docs/DOMAIN.md` (or add `docs/superpowers/specs/YYYY-MM-DD-<topic>.md`).
2. Get approval before implementing cross-cutting entity changes.
3. Then update domain → store → service → worker → api → cli → target in that order.