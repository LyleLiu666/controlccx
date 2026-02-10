#!/usr/bin/env bash
set -euo pipefail

# Manual acceptance helper for:
# 1) same-session continue queues
# 2) preempt orders C before B
# 3) queue visibility API
#
# Usage:
#   ./scripts/manual_session_queue_acceptance.sh c:<conversation_id>
# or
#   ./scripts/manual_session_queue_acceptance.sh s:<session_id>
#
# Optional env:
#   BASE_URL=http://127.0.0.1:5174
#   TOKEN=<instance-token>

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <session-key>" >&2
  exit 1
fi

BASE_URL="${BASE_URL:-http://127.0.0.1:5174}"
KEY="$1"
TOKEN="${TOKEN:-}"

H=()
if [[ -n "$TOKEN" ]]; then
  H=(-H "X-ControlCCX-Token: $TOKEN")
fi

echo "[1/4] continue B (queue when in-flight)"
curl -sS "${H[@]}" -X POST \
  -H 'Content-Type: application/json' \
  "${BASE_URL}/api/sessions/${KEY}/continue" \
  -d '{"prompt":"B"}' | jq .

echo
echo "[2/4] preempt-continue C (should outrank B)"
curl -sS "${H[@]}" -X POST \
  -H 'Content-Type: application/json' \
  "${BASE_URL}/api/sessions/${KEY}/preempt-continue" \
  -d '{"prompt":"C"}' | jq .

echo
echo "[3/4] queue snapshot"
curl -sS "${H[@]}" \
  "${BASE_URL}/api/sessions/${KEY}/queue" | jq .

echo
echo "[4/4] hint: observe task list/logs to confirm order A(cancel) -> C -> B"
echo "curl -sS ${BASE_URL}/api/tasks?limit=50 | jq ."
