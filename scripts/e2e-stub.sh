#!/usr/bin/env bash
# End-to-end suite against stub target (starts API + worker, tears down).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BOOTSTRAP="${LAUNCHPAD_BOOTSTRAP_TOKEN:-e2e-bootstrap-token}"
API_ADDR="${LAUNCHPAD_E2E_API_ADDR:-127.0.0.1:18080}"
API_URL="${LAUNCHPAD_API_URL:-http://${API_ADDR}}"
WORKDIR="${TMPDIR:-/tmp}/launchpad-e2e-stub-$$"
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

# Fixed AES-256 key (base64) for e2e secret config encryption (S2).
SECRETS_KEY="${LAUNCHPAD_SECRETS_KEY:-8+0PgKCW0812sx/4CpEQ2Rv18dQCZTRK7IyZQDwLQEk=}"

echo "==> start api (${API_ADDR})"
LAUNCHPAD_DATABASE_URL="${DB_URL}" \
LAUNCHPAD_AUTO_MIGRATE=true \
LAUNCHPAD_BOOTSTRAP_TOKEN="${BOOTSTRAP}" \
LAUNCHPAD_API_ADDR="${API_ADDR}" \
LAUNCHPAD_SECRETS_KEY="${SECRETS_KEY}" \
  ./bin/launchpad-api >"${API_LOG}" 2>&1 &
API_PID=$!

echo "==> start worker (stub only)"
LAUNCHPAD_DATABASE_URL="${DB_URL}" \
LAUNCHPAD_ENABLE_KUBERNETES=false \
LAUNCHPAD_SECRETS_KEY="${SECRETS_KEY}" \
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
  echo "api failed to become healthy; log:" >&2
  cat "${API_LOG}" >&2
  exit 1
}

echo "==> go test -tags=e2e"
export LAUNCHPAD_E2E=1
export LAUNCHPAD_API_URL="${API_URL}"
export LAUNCHPAD_BOOTSTRAP_TOKEN="${BOOTSTRAP}"
export LAUNCHPAD_E2E_TARGET=stub
export LAUNCHPAD_E2E_CLI="${ROOT}/bin/launchpad"
export LAUNCHPAD_E2E_IMAGE="${LAUNCHPAD_E2E_IMAGE:-e2e:stub}"

set +e
mise exec -- go test -tags=e2e ./test/e2e/ -count=1 -timeout=5m -v
STATUS=$?
set -e

if [[ "${STATUS}" -ne 0 ]]; then
  echo "==> api log" >&2
  cat "${API_LOG}" >&2 || true
  echo "==> worker log" >&2
  cat "${WORKER_LOG}" >&2 || true
fi
exit "${STATUS}"
