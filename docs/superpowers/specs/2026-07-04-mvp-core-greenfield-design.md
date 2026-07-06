# MVP Core Greenfield Build

| Field | Value |
|-------|-------|
| **Status** | Completed (merged 2026-07) |
| **Date** | 2026-07-04 |
| **Domain spec** | `docs/DOMAIN.md` |
| **Scope** | MVP core — greenfield rewrite of spike |

---

## Goal

Rebuilt Launchpad on the new domain model from scratch. Delivered a working solo-engineer loop:

```bash
launchpad projects create my-api
launchpad use my-api
launchpad config set PORT=3000
launchpad changeset add --image my-api:v1
launchpad changeset push
launchpad ps
launchpad releases
```

Single project, single `dev` environment, single service — but the **schema and types** reflect the full hierarchy so later phases extend without rework.

---

## Approaches Considered

### A. Layer-by-layer rewrite on a feature branch (recommended)

Delete spike domain/store/service code. Rebuild in dependency order with a commit per layer: domain → schema/store → services → target → worker → API → CLI.

**Pros:** Clean boundaries, reviewable commits, tests per layer.  
**Cons:** Temporary breakage mid-branch.

### B. Incremental rename/refactor in place

Rename `App` → `Project`, add tables, shim old API paths.

**Pros:** Always compiles.  
**Cons:** Carries spike debt; violates greenfield intent.

### C. New module path (`v2/internal`)

Run old and new side by side.

**Pros:** Zero downtime on spike.  
**Cons:** YAGNI for a greenfield project.

**Decision: A.** No compatibility shims. Spike code removed as layers land.

---

## MVP Scope

### In scope

| Area | MVP behavior |
|------|--------------|
| **Entities** | Workspace, Project, Environment (`dev`), Service, Process, Release, Deployment, Changeset, Job |
| **Bootstrap** | `POST /v1/projects` creates project + `dev` env + primary service + `web` process |
| **Config** | Service-scoped config vars in `dev` only (no workspace/shared layers) |
| **Changeset** | Project-scoped; changes target primary service; `push` creates release + deployment |
| **Deploy** | Image-only releases; async worker; K8s + stub targets |
| **Auth** | Bootstrap token + scoped API tokens |
| **CLI** | `projects create`, `use`, `config`, `changeset`, `deploy`, `ps`, `releases` |
| **API** | Projects, config, changeset, releases, processes, jobs, tokens, health |

### Explicitly deferred

| Feature | Phase |
|---------|-------|
| Multi-environment (`staging`, `prod`, promotion) | Phase 2 |
| Multi-service projects, ReleaseSet coordination | Phase 3 |
| Layered config (workspace, shared) | Phase 2 |
| Bindings / `${{ refs }}` | Phase 4 |
| Immediate `scale`, `rollback` commands | Post-MVP |
| `deployment_events`, SSE, cancel | Post-MVP |
| Idempotency keys | Post-MVP |
| Builds table / in-cluster build | v1.1 |
| OpenAPI, observability, Helm | Post-MVP |

### Removed from spike

- `App` entity and all `app_*` tables/API paths
- `scale_service`, rollback worker handlers, rollback API
- Root `migrations/` duplicate (consolidate under `internal/store/migrations/`)
- `002_changesets` migration (fold into `001`)
- Unused tables: `builds`, `deployment_events`, `idempotency_keys` (until needed)
- `DESIGN.md` entity sections remain marked superseded; no rewrite in this PR

---

## Domain Model (MVP subset)

```
Workspace
└── Project (primary_service = name)
    ├── Environment "dev" (target_type, target_config)
    ├── Service (same name as project)
    │   ├── Process "web" (quantity=1, expose=http)
    │   ├── ConfigVar[] (scoped to service + dev env)
    │   └── Release[] (per service)
    ├── Changeset (0..1 open)
    └── Deployment[] (service × environment)
```

### Context resolution (MVP)

- **Workspace:** from auth token
- **Project:** URL path `{project}` or CLI `launchpad use`
- **Environment:** hardcoded `dev` (header `X-Launchpad-Environment` accepted but only `dev` valid)
- **Service:** project's `primary_service`

### Changeset (MVP)

- `--service` flag accepted but must match `primary_service` (400 otherwise)
- Change types: `config`, `scale`, `image` only
- `push` → merge config, update process quantities, create release, enqueue deploy job
- No ReleaseSet table (single service = implicit single release)

### Release snapshot

```go
type Release struct {
    ServiceID        uuid.UUID
    Version          int
    ArtifactRef      string
    ConfigResolved   map[string]string  // service config at snapshot time
    ProcessSnapshot  map[string]ProcessSnapshot
    Status           ReleaseStatus
    Description      string
}
```

### Deployment concurrency

Partial unique index: one active deployment per `(service_id, environment_id)` where status ∈ (`pending`, `deploying`).

---

## API (MVP)

```
POST   /v1/projects
GET    /v1/projects
GET    /v1/projects/{project}

GET    /v1/projects/{project}/config
PATCH  /v1/projects/{project}/config

GET    /v1/projects/{project}/changeset
POST   /v1/projects/{project}/changeset/changes
DELETE /v1/projects/{project}/changeset
POST   /v1/projects/{project}/changeset/push

POST   /v1/projects/{project}/releases          # immediate deploy (--now equivalent)
GET    /v1/projects/{project}/releases
GET    /v1/projects/{project}/processes
GET    /v1/projects/{project}/deployments/{id}  # optional: latest active

POST   /v1/tokens
GET    /v1/jobs/{id}
GET    /healthz
```

All paths are workspace-scoped via auth token (no `{workspace}` in URL for MVP).

---

## Target Interface (MVP)

```go
type DeployRequest struct {
    Project     domain.Project
    Service     domain.Service
    Environment domain.Environment
    Release     domain.Release
    Processes   []domain.Process
    Config      map[string]string
}
```

K8s naming: `launchpad-{project}-{service}-{process}` in environment namespace.

---

## File Plan

| Action | Path |
|--------|------|
| Replace | `internal/domain/*` |
| Replace | `internal/store/*` + single `001_initial` migration |
| Replace | `internal/service/*` |
| Update | `internal/target/*`, `internal/jobs/worker.go` |
| Replace | `internal/api/handlers.go` |
| Replace | `internal/cli/root.go`, `pkg/apiclient/client.go` |
| Update | `cmd/api/main.go`, `cmd/worker/main.go`, `cmd/launchpad/main.go` |
| Update | `README.md` |
| Delete | `migrations/` (root), spike-only files |

---

## Commit Plan

1. `docs: add MVP greenfield design spec and implementation plan`
2. `feat(domain): add project/environment/service model types`
3. `feat(store): add schema and repositories for MVP model`
4. `feat(service): add project, config, release, and changeset services`
5. `feat(target): update deploy interface for service+environment`
6. `feat(worker): deploy job handler for new model`
7. `feat(api): REST handlers for MVP endpoints`
8. `feat(cli): client and commands for MVP workflow`
9. `chore: remove spike app model and unused code`

---

## Testing Strategy

- Domain FSM unit tests (deployment transitions)
- Store integration tests with SQLite (project bootstrap, release enqueue, job lease)
- Kubernetes target unit tests with fake clientset
- Worker test with stub target
- Changeset service test (stage + push materialization)

---

## Success Criteria

1. `make build && make test` pass
2. `make migrate-up && make run-api` + `make run-worker` start cleanly
3. End-to-end: create project → config → changeset push → deployment reaches `running` (stub target)
4. No references to `App`, `app_id`, or `/v1/apps` in code
5. README reflects new commands and model