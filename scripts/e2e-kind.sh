#!/usr/bin/env bash
# End-to-end suite against a kind Kubernetes cluster.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

need() { command -v "$1" >/dev/null || { echo "missing required command: $1" >&2; exit 1; }; }
need docker
need kind
need kubectl
need curl
need mise

CLUSTER="${LAUNCHPAD_E2E_KIND_CLUSTER:-launchpad-e2e}"
NAMESPACE="${LAUNCHPAD_E2E_NAMESPACE:-lp-e2e-$$}"
BOOTSTRAP="${LAUNCHPAD_BOOTSTRAP_TOKEN:-e2e-bootstrap-token}"
API_ADDR="${LAUNCHPAD_E2E_API_ADDR:-127.0.0.1:18080}"
API_URL="${LAUNCHPAD_API_URL:-http://${API_ADDR}}"
WORKDIR="${TMPDIR:-/tmp}/launchpad-e2e-kind-$$"
DB_FILE="${WORKDIR}/launchpad.db"
DB_URL="file:${DB_FILE}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
API_LOG="${WORKDIR}/api.log"
WORKER_LOG="${WORKDIR}/worker.log"
API_PID=""
WORKER_PID=""
CREATED_CLUSTER=0

cleanup() {
  if [[ -n "${API_PID}" ]] && kill -0 "${API_PID}" 2>/dev/null; then
    kill "${API_PID}" 2>/dev/null || true
    wait "${API_PID}" 2>/dev/null || true
  fi
  if [[ -n "${WORKER_PID}" ]] && kill -0 "${WORKER_PID}" 2>/dev/null; then
    kill "${WORKER_PID}" 2>/dev/null || true
    wait "${WORKER_PID}" 2>/dev/null || true
  fi
  if kubectl get ns "${NAMESPACE}" >/dev/null 2>&1; then
    kubectl delete ns "${NAMESPACE}" --wait=false >/dev/null 2>&1 || true
  fi
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

mkdir -p "${WORKDIR}"

if ! kind get clusters 2>/dev/null | grep -qx "${CLUSTER}"; then
  echo "==> kind create cluster --name ${CLUSTER}"
  kind create cluster --name "${CLUSTER}"
  CREATED_CLUSTER=1
else
  echo "==> reuse kind cluster ${CLUSTER}"
  kind export kubeconfig --name "${CLUSTER}" >/dev/null
fi

echo "==> namespace ${NAMESPACE}"
kubectl create namespace "${NAMESPACE}"

echo "==> build"
mise exec -- make build

echo "==> start api"
LAUNCHPAD_DATABASE_URL="${DB_URL}" \
LAUNCHPAD_AUTO_MIGRATE=true \
LAUNCHPAD_BOOTSTRAP_TOKEN="${BOOTSTRAP}" \
LAUNCHPAD_API_ADDR="${API_ADDR}" \
  ./bin/launchpad-api >"${API_LOG}" 2>&1 &
API_PID=$!

echo "==> start worker (kubernetes enabled)"
# Worker discovers kubeconfig from environment (kind export sets cluster context).
LAUNCHPAD_DATABASE_URL="${DB_URL}" \
  ./bin/launchpad-worker >"${WORKER_LOG}" 2>&1 &
WORKER_PID=$!

echo "==> wait for healthz"
for _ in $(seq 1 40); do
  curl -sf "${API_URL}/healthz" >/dev/null && break
  sleep 0.25
done
curl -sf "${API_URL}/healthz" >/dev/null || {
  cat "${API_LOG}" >&2
  exit 1
}

export LAUNCHPAD_E2E=1
export LAUNCHPAD_API_URL="${API_URL}"
export LAUNCHPAD_BOOTSTRAP_TOKEN="${BOOTSTRAP}"
export LAUNCHPAD_E2E_TARGET=kubernetes
export LAUNCHPAD_E2E_NAMESPACE="${NAMESPACE}"
export LAUNCHPAD_E2E_IMAGE="${LAUNCHPAD_E2E_IMAGE:-nginx:stable}"
export LAUNCHPAD_E2E_CLI="${ROOT}/bin/launchpad"
export LAUNCHPAD_E2E_TIMEOUT="${LAUNCHPAD_E2E_TIMEOUT:-3m}"

echo "==> go test -tags=e2e (kubernetes)"
set +e
mise exec -- go test -tags=e2e ./test/e2e/ -count=1 -timeout=15m -v
STATUS=$?
set -e

if [[ "${STATUS}" -ne 0 ]]; then
  echo "==> api log" >&2
  cat "${API_LOG}" >&2 || true
  echo "==> worker log" >&2
  cat "${WORKER_LOG}" >&2 || true
  kubectl -n "${NAMESPACE}" get all,secrets 2>&1 || true
fi
exit "${STATUS}"
