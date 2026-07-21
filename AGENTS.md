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
| 5 | `docs/FEATURE-DEVELOPMENT.md` | Starting a feature, branching, specs, plans, commits, PRs |
| 6 | `docs/AUTONOMOUS-MODE.md` | User-authorized autonomous / low-input multi-step agent work |

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

For behavioral changes touching deploy flow, also run stub e2e:

```bash
make e2e-stub          # real API + worker, stub target (canonical automated path)
```

Kind e2e (`make e2e-kind`) is optional/nightly — needs Docker, kind, and kubectl. Manual smoke notes remain in `.grok/skills/launchpad-dev/SKILL.md`.

## Code conventions

- **Go layout:** standard `cmd/` + `internal/` + `pkg/`. Keep domain free of SQL/HTTP.
- **Store:** use `?` placeholders; `rebind()` handles Postgres. Use `Transact()` for multi-step writes.
- **Errors:** `pkg/launchpad` sentinels (`ErrNotFound`, `ErrConflict`, `ErrBadRequest`, …); API returns `application/problem+json`.
- **Auth:** workspace-scoped tokens; bootstrap via `LAUNCHPAD_BOOTSTRAP_TOKEN`. Context key `team_id` is legacy naming for workspace ID.
- **Environment:** default `dev`; select via `X-Launchpad-Environment` / CLI `env use` / `LAUNCHPAD_ENV`.
- **Targets:** implement `internal/target.Target`; K8s resource prefix `launchpad-{project}-{service}-{process}`.
- **Commits:** focused, present tense; one logical layer per commit when possible.

## MVP scope boundaries

**In scope now:** multi-env (ambient header + CLI `env *`), layered config (shared + service), single primary service per project, image-only releases, implicit staging CLI, deploy `--wait`, rollback, promote, logs, inspect, release archaeology, doctor, project-local context, deploy worker, stub + K8s targets. Identity phase 1: service-account principals on tokens, release attribution, audit events.

**Deferred (do not half-build):** multi-service ReleaseSet, config bindings (`${{ refs }}`), workspace config layer, OIDC login (after principals phase 1), scale API (target-side), SSE/events, idempotency, builds, Helm. OpenAPI skeleton is shipped (`docs/openapi.yaml`); keep it in sync when adding routes.

**Designed runtime depth (implement via QUEUE, do not half-build snapshots without target wiring):** process commands + Procfile, portable health/readiness, release-immutable config materialization, target extensions + capabilities — `docs/superpowers/specs/2026-07-20-runtime-target-depth-design.md`.

If a task crosses a deferred boundary, update `docs/DOMAIN.md` or write a new spec in `docs/superpowers/specs/` first.

## Feature development workflow

For new features or multi-session work, follow `docs/FEATURE-DEVELOPMENT.md`:

1. **Spec** → `docs/superpowers/specs/YYYY-MM-DD-<name>-design.md` (templates in `docs/superpowers/templates/`)
2. **Plan** → `docs/superpowers/plans/YYYY-MM-DD-<name>.md`
3. **Branch** → `feat/<name>` (optionally in `.worktrees/` — gitignored)
4. **Implement** → one Go layer per commit; verify with `mise exec -- make test`
5. **PR** → link spec in description; keep worktree until review completes

Invoke `/launchpad-feature` at the start of agent-driven feature work.

## Project skills

| Skill | When to use |
|-------|-------------|
| `launchpad-feature` | New features, specs, plans, branching, long-horizon tasks |
| `launchpad-autonomous` | User-authorized ADM: named modes, DoD, worktrees, self-approve recommended path, subagents |
| `launchpad-domain` | Entity changes, API design, invariants |
| `launchpad-dev` | Build, test, local API/worker, smoke deploy |

Invoke via `/launchpad-feature`, `/launchpad-autonomous`, `/launchpad-domain`, `/launchpad-dev`, or let auto-invocation match the skill description.

**ADM protocol:** `docs/AUTONOMOUS-MODE.md` — only when the user explicitly authorizes autonomous mode. Named modes: single-feature (default), integration-stack, queue-drain. Work queue / ideas / persona: `docs/superpowers/program/`. Snapshot helper: `scripts/adm-status`.

## Suggested future tooling (not yet implemented)

| Tool | Value |
|------|-------|
| **Launchpad MCP server** | Agents call the REST API (create project, push changeset) without shell curl |
| **`launchpad-target` skill** | Checklist for adding Nomad/ECS backends to `internal/target/` |

**Implemented:** CI on PRs (`.github/workflows/ci.yml` — unit tests + `make e2e-stub`); kind e2e nightly/label (`.github/workflows/e2e-kind.yml`); e2e harness (`make e2e-stub` / `make e2e-kind`, `test/e2e`, `scripts/e2e-*.sh`).

## What not to do

- Do not add `App` types, `app_id` columns, or `/v1/apps` routes.
- Do not skip `docs/DOMAIN.md` when changing the mental model.
- Do not run drive-by refactors unrelated to the task.
- Do not commit `*.db`, `.env`, or `bin/` artifacts.
- Do not assume Heroku API parity is required.