# Launchpad

Heroku/Deis-style deployment platform with a project/environment/service domain model, asynchronous worker, and pluggable targets (Kubernetes and stub).

## Architecture

- **API server** (`cmd/api`) — REST control plane, enqueues deploy jobs
- **Worker** (`cmd/worker`) — leases jobs from Postgres/SQLite, applies releases to targets
- **CLI** (`cmd/launchpad`) — manages projects, config, changesets, and releases

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
launchpad use my-api
launchpad config set PORT=3000
launchpad changeset add --image my-api:v1
launchpad changeset push
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

## Changeset workflow

Stage multiple changes, then push as a single release:

```bash
launchpad changeset add PORT=3000
launchpad changeset add --scale web=3 --image my-api:v2
launchpad changeset status
launchpad changeset push --message "Scale web + update config"
launchpad changeset reset
```

Immediate deploy (bypasses changeset):

```bash
launchpad deploy --image my-api:v1
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

## CLI configuration

| Variable | Description |
|----------|-------------|
| `LAUNCHPAD_API_URL` | API base URL (default `http://localhost:8080`) |
| `LAUNCHPAD_TOKEN` | Bearer token |
| `LAUNCHPAD_PROJECT` | Active project (overrides `~/.launchpad/config`) |

`launchpad use <project>` writes the active project to `~/.launchpad/config`.

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
POST   /v1/projects/{project}/releases
GET    /v1/projects/{project}/releases
GET    /v1/projects/{project}/processes
GET    /v1/jobs/{id}
POST   /v1/tokens
GET    /healthz
```

## Design documents

- **[Domain model](docs/DOMAIN.md)** — entity hierarchy, config layers, changeset semantics
- **[MVP greenfield spec](docs/superpowers/specs/2026-07-04-mvp-core-greenfield-design.md)** — approved MVP scope and file plan
- **[System design](docs/DESIGN.md)** — control plane architecture (entity sections superseded by DOMAIN.md)