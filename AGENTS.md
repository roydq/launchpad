# AGENTS.md

Guidance for AI agents and coding assistants working in this repository.

## What this project is

Launchpad is a self-hosted deployment control plane: Heroku-style developer experience (projects, config, releases, changesets) with pluggable runtime backends (Kubernetes, stub). Long-running deploys run asynchronously via a worker; the API returns quickly with job IDs.

**North star:** `docs/DOMAIN.md` — the domain model is the product. Start from DX, then adapt API, store, worker, CLI, and targets to match.

## Authoritative docs (read order)

| Priority | Document | Use when |
|----------|----------|----------|
| 1 | `docs/DOMAIN.md` | Any entity, API shape, lifecycle, or invariant question |
| 2 | `docs/superpowers/specs/2026-07-04-mvp-core-greenfield-design.md` | Current MVP scope and what's deferred |
| 3 | `README.md` | Running locally, CLI examples |
| 4 | `docs/DESIGN.md` | Control-plane architecture, jobs, auth, roadmap |

Do not reintroduce the old `App` model, `/v1/apps` routes, or per-environment duplicate apps.

## Architecture

```
cmd/api        → REST + auth, enqueues jobs
cmd/worker     → leases jobs, runs deployment FSM, calls targets
cmd/launchpad  → CLI (cobra)
cmd/migrate    → SQL migrations

internal/domain/    → types, FSM, invariants (no I/O)
internal/store/     → SQL repos, migrations (Postgres + SQLite)
internal/service/   → business logic
internal/api/       → HTTP handlers (chi, RFC 7807 errors)
internal/jobs/      → worker loop
internal/target/    → Target interface + kubernetes/ + stub/
pkg/apiclient/      → Go HTTP client for CLI
pkg/launchpad/      → shared errors
```

**MVP domain hierarchy:**

```
Workspace → Project → Environment ("dev") + Service → Process
Changeset (project-scoped) → Release (service-scoped) → Deployment (service × env)
```

Bootstrap on `POST /v1/projects`: creates project, `dev` environment, primary service (same name), `web` process.

## Toolchain

Go is managed by **mise** (`mise.toml`, Go 1.26). Agent shells may not have `go` on PATH.

```bash
mise trust          # once per machine, if prompted
mise install
mise exec -- make test
mise exec -- make build
```

Prefer `mise exec --` over assuming `go` is available.

## Verification before claiming done

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
```

For behavioral changes touching deploy flow, also run the stub-target smoke path (see `.grok/skills/launchpad-dev/SKILL.md`).

## Code conventions

- **Go layout:** standard `cmd/` + `internal/` + `pkg/`. Keep domain free of SQL/HTTP.
- **Store:** use `?` placeholders; `rebind()` handles Postgres. Use `Transact()` for multi-step writes.
- **Errors:** `pkg/launchpad` sentinels (`ErrNotFound`, `ErrConflict`, `ErrBadRequest`, …); API returns `application/problem+json`.
- **Auth:** workspace-scoped tokens; bootstrap via `LAUNCHPAD_BOOTSTRAP_TOKEN`. Context key `team_id` is legacy naming for workspace ID.
- **MVP environment:** hardcode or default `dev` until multi-env phase lands.
- **Targets:** implement `internal/target.Target`; K8s resource prefix `launchpad-{project}-{service}-{process}`.
- **Commits:** focused, present tense; one logical layer per commit when possible.

## MVP scope boundaries

**In scope now:** single `dev` environment, single primary service per project, image-only releases, changesets, deploy worker, stub + K8s targets.

**Deferred (do not half-build):** multi-environment promotion, multi-service ReleaseSet, config bindings (`${{ refs }}`), workspace/shared config layers, scale/rollback APIs, SSE/events, idempotency, builds, OpenAPI, Helm.

If a task crosses a deferred boundary, update `docs/DOMAIN.md` or write a new spec in `docs/superpowers/specs/` first.

## Project skills

| Skill | When to use |
|-------|-------------|
| `launchpad-domain` | Entity changes, API design, invariants |
| `launchpad-dev` | Build, test, local API/worker, smoke deploy |

Invoke via `/launchpad-domain`, `/launchpad-dev`, or let auto-invocation match the skill description.

## Suggested future tooling (not yet implemented)

| Tool | Value |
|------|-------|
| **Launchpad MCP server** | Agents call the REST API (create project, push changeset) without shell curl |
| **`launchpad-target` skill** | Checklist for adding Nomad/ECS backends to `internal/target/` |
| **CI workflow** | `mise exec -- make test` on PRs |
| **dev smoke script** | `scripts/smoke-stub.sh` — api + worker + deploy assertion |

## What not to do

- Do not add `App` types, `app_id` columns, or `/v1/apps` routes.
- Do not skip `docs/DOMAIN.md` when changing the mental model.
- Do not run drive-by refactors unrelated to the task.
- Do not commit `*.db`, `.env`, or `bin/` artifacts.
- Do not assume Heroku API parity is required.