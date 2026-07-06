#!/usr/bin/env bash
# Smoke test against a running API + worker (stub target).
# Usage: LAUNCHPAD_BOOTSTRAP_TOKEN=dev-bootstrap-token ./scripts/smoke-stub.sh
set -euo pipefail

API_URL="${LAUNCHPAD_API_URL:-http://localhost:8080}"
BOOTSTRAP="${LAUNCHPAD_BOOTSTRAP_TOKEN:?set LAUNCHPAD_BOOTSTRAP_TOKEN}"
PROJECT="smoke-$(date +%s)"

echo "==> healthz"
curl -sf "$API_URL/healthz" >/dev/null

echo "==> create token"
TOKEN_RESP=$(curl -sf -X POST "$API_URL/v1/tokens" \
  -H "Authorization: Bearer $BOOTSTRAP" \
  -H "Content-Type: application/json" \
  -d '{"name":"smoke","workspace":"default","scopes":["admin","project:write","project:read","deploy"]}')
TOKEN=$(echo "$TOKEN_RESP" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [[ -z "$TOKEN" ]]; then
  echo "failed to parse token from: $TOKEN_RESP" >&2
  exit 1
fi

AUTH="Authorization: Bearer $TOKEN"

echo "==> create project $PROJECT"
curl -sf -X POST "$API_URL/v1/projects" \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"name\":\"$PROJECT\",\"target\":{\"type\":\"stub\",\"namespace\":\"default\"}}" >/dev/null

echo "==> deploy"
DEPLOY_RESP=$(curl -sf -X POST "$API_URL/v1/projects/$PROJECT/releases" \
  -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"source":{"type":"image","image":"smoke:v1"},"description":"smoke test"}')
JOB_ID=$(echo "$DEPLOY_RESP" | sed -n 's/.*"id":"\([^"]*\)".*"type":"deploy".*/\1/p' | head -1)
if [[ -z "$JOB_ID" ]]; then
  JOB_ID=$(echo "$DEPLOY_RESP" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
fi

echo "==> poll job $JOB_ID"
for _ in $(seq 1 30); do
  STATUS=$(curl -sf -H "$AUTH" "$API_URL/v1/jobs/$JOB_ID" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')
  echo "    status=$STATUS"
  [[ "$STATUS" == "succeeded" ]] && break
  [[ "$STATUS" == "failed" || "$STATUS" == "dead" ]] && exit 1
  sleep 0.5
done

echo "==> processes"
curl -sf -H "$AUTH" "$API_URL/v1/projects/$PROJECT/processes"
echo
echo "==> smoke passed"