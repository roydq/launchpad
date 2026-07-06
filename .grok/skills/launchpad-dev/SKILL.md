---
name: launchpad-dev
description: >
  Launchpad local development and verification. Use when building, testing,
  running API/worker, smoke-testing deploys, or debugging jobs. Use when go is
  missing from PATH (use mise). Triggers on "make test", "run api", "run worker",
  "smoke test", "local dev", "mise", "verify launchpad".
---

# Launchpad Dev Workflow

## Toolchain

Go is pinned in `mise.toml`. **Always use mise** in agent shells:

```bash
cd <repo-root>
mise trust    # if prompted
mise install
mise exec -- go version
```

## Build and test

```bash
mise exec -- make build
mise exec -- make test
mise exec -- go vet ./...
mise exec -- make lint   # golangci-lint if installed, else go vet
```

Run tests after any change to `internal/domain`, `internal/store`, `internal/service`, `internal/jobs`, or `internal/target`.

## Run locally (two terminals)

**Terminal 1 — API:**

```bash
make migrate-up
LAUNCHPAD_BOOTSTRAP_TOKEN=dev-bootstrap-token make run-api
```

**Terminal 2 — Worker:**

```bash
LAUNCHPAD_DATABASE_URL="file:launchpad.db?_pragma=foreign_keys(1)" make run-worker
```

API auto-migrates when `LAUNCHPAD_AUTO_MIGRATE=true` (set by `run-api`).

## Smoke test (stub target)

With API + worker running:

```bash
# 1. Create admin token
curl -s -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer dev-bootstrap-token" \
  -H "Content-Type: application/json" \
  -d '{"name":"smoke","workspace":"default","scopes":["admin","project:write","project:read","deploy"]}'

# 2. Use token (replace TOKEN)
export LAUNCHPAD_TOKEN=lp_...

# 3. CLI path (after make build)
./bin/launchpad projects create smoke-demo --target stub --namespace default
./bin/launchpad use smoke-demo
./bin/launchpad deploy --image smoke:v1

# 4. Poll job / check processes
curl -s -H "Authorization: Bearer $LAUNCHPAD_TOKEN" \
  http://localhost:8080/v1/projects/smoke-demo/processes
```

Or run `scripts/smoke-stub.sh` if present (requires API + worker already running).

## Kubernetes target

Worker auto-registers K8s when kubeconfig is available. Disable with `LAUNCHPAD_ENABLE_KUBERNETES=false`.

Create projects with `--target kubernetes --namespace <ns>`. Resources use prefix `launchpad-{project}-{service}-{process}`.

## Common env vars

| Variable | Purpose |
|----------|-------------|
| `LAUNCHPAD_DATABASE_URL` | SQLite file or Postgres DSN |
| `LAUNCHPAD_BOOTSTRAP_TOKEN` | First-run admin bootstrap |
| `LAUNCHPAD_API_ADDR` | API listen (default `:8080`) |
| `LAUNCHPAD_ENABLE_KUBERNETES` | `false` to use stub only |

## Debugging deploy failures

1. `GET /v1/jobs/{id}` — job status and `last_error`
2. Worker logs — deployment FSM transitions
3. Store: `deployments` row for service + `dev` environment
4. Stub target always succeeds; K8s failures are usually RBAC, image pull, or readiness timeout

## Before claiming work complete

- [ ] `mise exec -- make test` passes
- [ ] `mise exec -- make build` passes
- [ ] Deploy flow verified on stub if worker/service/target changed