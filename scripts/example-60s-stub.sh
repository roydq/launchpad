#!/usr/bin/env bash
# 60s path: build, start API+worker (stub), create project, deploy --wait, inspect.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BOOTSTRAP="${LAUNCHPAD_BOOTSTRAP_TOKEN:-dev-bootstrap-token}"
API_ADDR="${LAUNCHPAD_E2E_API_ADDR:-127.0.0.1:18081}"
API_URL="http://${API_ADDR}"
WORKDIR="${TMPDIR:-/tmp}/launchpad-60s-$$"
DB_FILE="${WORKDIR}/launchpad.db"
DB_URL="file:${DB_FILE}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
API_LOG="${WORKDIR}/api.log"
WORKER_LOG="${WORKDIR}/worker.log"
API_PID=""
WORKER_PID=""

cleanup() {
  if [[ -n "${API_PID}" ]] && kill -0 "${API_PID}" 2>/dev/null; then
    kill "${API_PID}" 2>/dev/null || true
    wait "${API_PID}" 2>/dev/null || true
  fi
  if [[ -n "${WORKER_PID}" ]] && kill -0 "${WORKER_PID}" 2>/dev/null; then
    kill "${WORKER_PID}" 2>/dev/null || true
    wait "${WORKER_PID}" 2>/dev/null || true
  fi
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

mkdir -p "${WORKDIR}"
echo "==> build"
mise exec -- make build

echo "==> start api (${API_ADDR})"
LAUNCHPAD_DATABASE_URL="${DB_URL}" \
LAUNCHPAD_AUTO_MIGRATE=true \
LAUNCHPAD_BOOTSTRAP_TOKEN="${BOOTSTRAP}" \
LAUNCHPAD_API_ADDR="${API_ADDR}" \
LAUNCHPAD_ENABLE_KUBERNETES=false \
  ./bin/launchpad-api >"${API_LOG}" 2>&1 &
API_PID=$!

echo "==> start worker"
LAUNCHPAD_DATABASE_URL="${DB_URL}" \
LAUNCHPAD_ENABLE_KUBERNETES=false \
  ./bin/launchpad-worker >"${WORKER_LOG}" 2>&1 &
WORKER_PID=$!

echo "==> wait for healthz"
for _ in $(seq 1 40); do
  if curl -sf "${API_URL}/healthz" >/dev/null; then
    break
  fi
  sleep 0.25
done
curl -sf "${API_URL}/healthz" >/dev/null || {
  echo "api failed:" >&2
  cat "${API_LOG}" >&2
  exit 1
}

echo "==> mint token"
TOKEN_JSON=$(curl -s -X POST "${API_URL}/v1/tokens" \
  -H "Authorization: Bearer ${BOOTSTRAP}" \
  -H "Content-Type: application/json" \
  -d '{"name":"60s","workspace":"default","scopes":["admin","project:write","project:read","deploy"]}')
TOKEN=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['token'])" "${TOKEN_JSON}")

export LAUNCHPAD_API_URL="${API_URL}"
export LAUNCHPAD_TOKEN="${TOKEN}"
# Isolate config and avoid inheriting caller project/env context.
unset LAUNCHPAD_PROJECT || true
unset LAUNCHPAD_ENV || true
export HOME="${WORKDIR}/home"
mkdir -p "${HOME}"
# Run CLI from an empty dir so walk-up .launchpad/config cannot pick the
# developer's real ~/.launchpad/config as a "project-local" override.
CLI_CWD="${WORKDIR}/cwd"
mkdir -p "${CLI_CWD}"
LP="${ROOT}/bin/launchpad"
NAME="hello-stub-$$"
echo "==> create + deploy ${NAME}"
(
  cd "${CLI_CWD}"
  ${LP} projects create "${NAME}" --target stub --namespace default
  ${LP} use "${NAME}"
  ${LP} deploy --image hello:v1 --wait --timeout 45s
  ${LP} releases list | head -40
  ${LP} inspect
  ${LP} diff
)

echo ""
echo "OK: 60s path succeeded for project ${NAME}"
echo "See examples/hello-stub/README.md for the manual walkthrough."
