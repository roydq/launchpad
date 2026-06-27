# Launchpad

Heroku/Deis-style application deployment API with an asynchronous worker backend and pluggable deployment targets (Kubernetes first).

## Architecture

- **API server** (`cmd/api`) — REST control plane, enqueues long-running work
- **Worker** (`cmd/worker`) — leases jobs from a Postgres/SQLite queue, runs deployment state machines
- **CLI** (`cmd/launchpad`) — manages apps, config, and releases via the API

## Kubernetes target

Apps with `target.type: kubernetes` deploy to a shared namespace (set in `target.namespace`):

```json
{
  "name": "my-api",
  "target": {
    "type": "kubernetes",
    "namespace": "launchpad-apps",
    "cluster": "minikube"
  }
}
```

The worker registers the Kubernetes target automatically when a kubeconfig is available (`~/.kube/config`, `KUBECONFIG`, in-cluster, or `LAUNCHPAD_KUBECONFIG`). Set `LAUNCHPAD_ENABLE_KUBERNETES=false` to disable.

Resources created per app:

| Resource | Name |
|----------|------|
| Secret (config vars) | `launchpad-{app}-config` |
| Deployment (per process) | `launchpad-{app}-web` |
| Service (web process) | `launchpad-{app}-web` |

## Changeset workflow (git-like batching)

Stage multiple changes, then push them as a single deploy:

```bash
# Stage config, scale, and image changes (like git add)
launchpad changeset add PORT=3000 DATABASE_URL=postgres://...
launchpad changeset add --scale web=3 --image myapp:v2

# Review staged changes (like git status)
launchpad changeset status

# Apply everything in one release (like git push)
launchpad changeset push --message "Scale web + update config"

# Discard staging area (like git reset)
launchpad changeset reset
```

Immediate operations (bypass staging):

```bash
launchpad scale web=3
launchpad rollback 2
```

## Quick start

```bash
make build
make migrate-up
LAUNCHPAD_BOOTSTRAP_TOKEN=dev-bootstrap-token make run-api   # terminal 1
LAUNCHPAD_DATABASE_URL="file:launchpad.db" make run-worker   # terminal 2

# Create a token and app
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer dev-bootstrap-token" \
  -H "Content-Type: application/json" \
  -d '{"name":"cli","team":"default","scopes":["admin"]}'
```

## Design document

See `/tmp/grok-design-doc-a62c6260.md` for the full system design, API spec, and PR plan.