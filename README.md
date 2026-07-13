# Launchpad

Heroku/Deis-style deployment platform with a project/environment/service domain model, asynchronous worker, and pluggable targets (Kubernetes and stub).

## Architecture

- **API server** (`cmd/api`) — REST control plane, enqueues deploy jobs
- **Worker** (`cmd/worker`) — leases jobs from Postgres/SQLite, applies releases to targets
- **CLI** (`cmd/launchpad`) — manages projects, config, releases; stages changes implicitly

## Domain model (MVP)

```
Workspace
└── Project (primary_service = name)
    ├── Environment "dev"
    ├── Service (same name as project)
    │   ├── Process "web"
    │   └── Config vars (service + dev env)
    └── Changeset (0..1 open)
```

## Solo-engineer workflow

```bash
launchpad projects create my-api
launchpad use my-api                  # environment defaults to dev
launchpad config set PORT=3000
launchpad image my-api:v1
launchpad diff
launchpad deploy -m "initial"

# Second environment
launchpad env create staging --target stub
launchpad env use staging
launchpad config set LOG_LEVEL=info
launchpad deploy --image my-api:v1 -m "staging"

launchpad ps
launchpad releases
```

## Kubernetes target

Projects bootstrap a `dev` environment. Set target type on create:

```bash
launchpad projects create my-api --target kubernetes --namespace launchpad-apps
```

The worker registers Kubernetes when a kubeconfig is available (`~/.kube/config`, `KUBECONFIG`, in-cluster, or `LAUNCHPAD_KUBECONFIG`). Set `LAUNCHPAD_ENABLE_KUBERNETES=false` to disable.

Resources created per service:

| Resource | Name |
|----------|------|
| Secret (config) | `launchpad-{project}-{service}-config` |
| Deployment (per process) | `launchpad-{project}-{service}-{process}` |
| Service (http processes) | `launchpad-{project}-{service}-{process}` |

## Staging and deploy

Mutations stage by default into the project's open changeset (HTTP API still uses `/changeset*`). Review, then submit with `deploy`:

```bash
launchpad config set PORT=3000
launchpad scale web=3
launchpad image my-api:v2
launchpad status
launchpad diff
launchpad deploy -m "Scale web + update config"
launchpad reset   # discard pending without deploying
```

One-shot (append mutations and deploy):

```bash
launchpad deploy --image my-api:v1 PORT=8080 -m "bump"
```

Wait for the worker to finish the job:

```bash
launchpad deploy --image my-api:v1 --wait
launchpad deploy --image my-api:v1 --wait --timeout 2m
```

Rollback (new release from prior version; config re-resolved for current env):

```bash
launchpad rollback 1 --wait
```

Health check:

```bash
launchpad doctor
```

Process logs (current env; default process `web`):

```bash
launchpad logs
launchpad logs web
```

One-page project@env view:

```bash
launchpad inspect
```

Immediate release when staging is empty (`--now` on mutation commands only):

```bash
launchpad config set DEBUG=true --now -m "debug on"
```

## Prerequisites

Go is managed with [mise](https://mise.jdx.dev/). From the repo root:

```bash
mise trust          # first time only, if prompted
mise install        # installs Go 1.26 per mise.toml
```

With mise activated in your shell (`mise activate`), `go` and `make build` work normally. Otherwise prefix commands with `mise exec --`, e.g. `mise exec -- make test`.

## Quick start

```bash
make build
make migrate-up
LAUNCHPAD_BOOTSTRAP_TOKEN=dev-bootstrap-token make run-api   # terminal 1
LAUNCHPAD_DATABASE_URL="file:launchpad.db" make run-worker   # terminal 2

# Create an API token
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer dev-bootstrap-token" \
  -H "Content-Type: application/json" \
  -d '{"name":"cli","workspace":"default","scopes":["admin"]}'

export LAUNCHPAD_TOKEN=lp_...
launchpad projects create demo --target stub --namespace default
launchpad use demo
launchpad config set PORT=8080
launchpad deploy --image demo:v1
```

## End-to-end tests

Default unit tests never run e2e (`//go:build e2e`).

```bash
# Fast: real API + worker, stub target (no cluster)
make e2e-stub

# Slow: kind Kubernetes cluster + public nginx image
# Requires: Docker, kind, kubectl
make e2e-kind
```

Environment knobs: `LAUNCHPAD_E2E_IMAGE`, `LAUNCHPAD_E2E_NAMESPACE`, `LAUNCHPAD_E2E_TIMEOUT` (see `docs/superpowers/specs/2026-07-09-e2e-testing-design.md`).

## CLI configuration

| Variable | Description |
|----------|-------------|
| `LAUNCHPAD_API_URL` | API base URL (default `http://localhost:8080`) |
| `LAUNCHPAD_TOKEN` | Bearer token |
| `LAUNCHPAD_PROJECT` | Active project (overrides `~/.launchpad/config`) |
| `LAUNCHPAD_ENV` | Active environment (overrides config file; default `dev`) |

`launchpad use <project>` and `launchpad env use <name>` write sticky context to `~/.launchpad/config`. API calls send `X-Launchpad-Environment`.

Optional **project-local** file (walk parents from cwd):

```json
// .launchpad/config
{ "project": "my-api", "environment": "staging" }
```

Precedence: `LAUNCHPAD_PROJECT` / `LAUNCHPAD_ENV` → `.launchpad/config` → `~/.launchpad/config` → env default `dev`.  
`launchpad context` prints the resolved stack.

## API (MVP)

```
POST   /v1/projects
GET    /v1/projects
GET    /v1/projects/{project}
GET    /v1/projects/{project}/config
PATCH  /v1/projects/{project}/config
GET    /v1/projects/{project}/environments
POST   /v1/projects/{project}/environments
GET    /v1/projects/{project}/environments/{name}
GET    /v1/projects/{project}/changeset
POST   /v1/projects/{project}/changeset/changes
DELETE /v1/projects/{project}/changeset
POST   /v1/projects/{project}/changeset/push
POST   /v1/projects/{project}/releases
GET    /v1/projects/{project}/releases
GET    /v1/projects/{project}/processes
GET    /v1/jobs/{id}
POST   /v1/tokens
GET    /healthz
```

Header `X-Launchpad-Environment` (default `dev`) scopes config, changeset, and deploy routes.

## For AI agents

See **[AGENTS.md](AGENTS.md)** for domain conventions, toolchain, MVP scope, and project skills (`launchpad-domain`, `launchpad-dev`).

## Design documents

- **[Domain model](docs/DOMAIN.md)** — entity hierarchy, invariants, roadmap
- **[System design](docs/DESIGN.md)** — control plane architecture, API, storage, jobs
- **[AGENTS.md](AGENTS.md)** — contributor and AI agent conventions