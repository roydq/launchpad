# Hello Launchpad (stub target)

Zero-to-running on the **stub** target — the fastest dogfood path (no Kubernetes).

## 60-second path

From the **repository root** (API + worker must not already own the ports you use):

```bash
./scripts/example-60s-stub.sh
```

Or manually:

```bash
# terminal 1
export LAUNCHPAD_BOOTSTRAP_TOKEN=dev-bootstrap-token
export LAUNCHPAD_AUTO_MIGRATE=true
export LAUNCHPAD_ENABLE_KUBERNETES=false
make build && make run-api

# terminal 2
export LAUNCHPAD_DATABASE_URL="file:launchpad.db?_pragma=foreign_keys(1)"
export LAUNCHPAD_ENABLE_KUBERNETES=false
make run-worker

# terminal 3
export LAUNCHPAD_API_URL=http://127.0.0.1:8080
TOKEN=$(curl -s -X POST "$LAUNCHPAD_API_URL/v1/tokens" \
  -H "Authorization: Bearer dev-bootstrap-token" \
  -H "Content-Type: application/json" \
  -d '{"name":"local","workspace":"default","scopes":["admin","project:write","project:read","deploy"]}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
export LAUNCHPAD_TOKEN="$TOKEN"

./bin/launchpad projects create hello-stub --target stub
./bin/launchpad use hello-stub
./bin/launchpad deploy --image hello:v1 --wait
./bin/launchpad releases
./bin/launchpad inspect
./bin/launchpad diff
```

## What you should see

- Project + `dev` environment + primary service bootstrap
- Release v1 `succeeded` after worker runs
- `launchpad diff` → no pending changes

## Next steps

- `launchpad env create staging --target stub` then promote (see README promote section)
- Point an environment at Kubernetes with `--target kubernetes` when ready
