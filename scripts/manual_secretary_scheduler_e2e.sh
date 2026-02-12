#!/usr/bin/env bash
set -euo pipefail

# Manual end-to-end acceptance helper for secretary scheduler callback flow:
# 1) create a success scheduler (tasks_count)
# 2) create a failure scheduler (task_new_submit with missing required fields)
# 3) verify real-time SSE events:
#    - secretary.thinking(tool_result) for both success and failure
#    - secretary.message immediate callback reports for both
#
# This script uses the REAL interaction chain only:
#   /api/secretary/messages  -> LLM decides tool calls
#   /api/events              -> observe secretary.thinking / secretary.message
# Prerequisite:
#   Secretary backend LLM must be configured and available.
#
# Usage:
#   ./scripts/manual_secretary_scheduler_e2e.sh
#
# Optional env:
#   BASE_URL=http://127.0.0.1:5174
#   TOKEN=<instance-token>
#   TIMEOUT_SEC=90
#   INTERVAL_SEC=2
#   TTL_SEC=25

BASE_URL="${BASE_URL:-http://127.0.0.1:5174}"
TOKEN="${TOKEN:-}"
TIMEOUT_SEC="${TIMEOUT_SEC:-90}"
INTERVAL_SEC="${INTERVAL_SEC:-2}"
TTL_SEC="${TTL_SEC:-25}"

if [[ -z "$TOKEN" && -f "${HOME}/.controlccx/instance.token" ]]; then
  TOKEN="$(tr -d '[:space:]' <"${HOME}/.controlccx/instance.token" || true)"
fi

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "missing required command: $cmd" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd jq
require_cmd awk

if ! [[ "$TIMEOUT_SEC" =~ ^[0-9]+$ ]] || (( TIMEOUT_SEC <= 0 )); then
  echo "TIMEOUT_SEC must be a positive integer, got: $TIMEOUT_SEC" >&2
  exit 1
fi
if ! [[ "$INTERVAL_SEC" =~ ^[0-9]+$ ]] || (( INTERVAL_SEC <= 0 )); then
  echo "INTERVAL_SEC must be a positive integer, got: $INTERVAL_SEC" >&2
  exit 1
fi
if ! [[ "$TTL_SEC" =~ ^[0-9]+$ ]] || (( TTL_SEC <= 0 )); then
  echo "TTL_SEC must be a positive integer, got: $TTL_SEC" >&2
  exit 1
fi

HDR=()
if [[ -n "$TOKEN" ]]; then
  HDR=(-H "X-ControlCCX-Token: $TOKEN")
fi

EVENTS_JSONL="$(mktemp -t ccx_scheduler_e2e_events.XXXXXX)"
EVENTS_PID=""

cleanup() {
  if [[ -n "$EVENTS_PID" ]]; then
    kill "$EVENTS_PID" >/dev/null 2>&1 || true
    wait "$EVENTS_PID" 2>/dev/null || true
  fi
  rm -f "$EVENTS_JSONL"
}
trap cleanup EXIT

post_secretary() {
  local message="$1"
  local payload
  payload="$(jq -cn --arg message "$message" '{message:$message}')"
  curl -sS --fail-with-body "${HDR[@]}" \
    -X POST \
    -H "Content-Type: application/json" \
    "${BASE_URL}/api/secretary/messages" \
    -d "$payload"
}

wait_for_jq_event() {
  local jq_expr="$1"
  local timeout_sec="$2"

  local started deadline line
  started="$(date +%s)"
  deadline=$(( started + timeout_sec ))

  while (( "$(date +%s)" <= deadline )); do
    line="$(jq -c "select(${jq_expr})" "$EVENTS_JSONL" 2>/dev/null | tail -n 1 || true)"
    if [[ -n "$line" ]]; then
      echo "$line"
      return 0
    fi
    sleep 1
  done
  return 1
}

extract_schedule_id_by_target() {
  local target_tool="$1"
  jq -r --arg target "$target_tool" '
    select(.type=="secretary.thinking")
    | .payload as $p
    | select(($p.source // "")=="timer")
    | select(($p.target_tool_name // "")==$target)
    | select(($p.schedule_id // "")!="")
    | ($p.schedule_id // "")
  ' "$EVENTS_JSONL" 2>/dev/null | tail -n 1
}

wait_for_schedule_id_by_target() {
  local target_tool="$1"
  local timeout_sec="$2"
  local started deadline id

  started="$(date +%s)"
  deadline=$(( started + timeout_sec ))

  while (( "$(date +%s)" <= deadline )); do
    id="$(extract_schedule_id_by_target "$target_tool" || true)"
    if [[ -n "$id" ]]; then
      echo "$id"
      return 0
    fi
    sleep 1
  done
  return 1
}

echo "[0/7] quick server check"
curl -sS --fail-with-body "${HDR[@]}" "${BASE_URL}/api/system" >/dev/null

echo "[1/7] clear old secretary history"
curl -sS --fail-with-body "${HDR[@]}" \
  -X POST \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/secretary/clear" \
  -d '{}' >/dev/null

echo "[2/7] start SSE capture: ${BASE_URL}/api/events"
curl -sS -N "${HDR[@]}" -H "Accept: text/event-stream" "${BASE_URL}/api/events" \
  | awk '
      /^data: /{
        sub(/^data: /, "");
        print;
        fflush();
      }
    ' >>"$EVENTS_JSONL" &
EVENTS_PID="$!"

sleep 1

echo "[3/7] prime secretary callback style (immediate short report)"
post_secretary "从现在开始，凡是定时调度回调（成功或失败），请立刻给我一句简短中文汇报。" | jq -r '.reply // ""' | sed 's/^/  secretary> /'

echo "[4/7] create SUCCESS scheduler (tasks_count)"
SUCCESS_PROMPT="$(cat <<EOF
请调用 scheduler_create 创建一个调度任务，参数如下：
- tool_name: tasks_count
- tool_fields_json: {}
- interval_sec: ${INTERVAL_SEC}
- ttl_sec: ${TTL_SEC}
- allow_write: false
创建后请简单确认。
EOF
)"
post_secretary "$SUCCESS_PROMPT" | jq -r '.reply // ""' | sed 's/^/  secretary> /'

echo "  waiting success schedule_id..."
SUCCESS_SID="$(wait_for_schedule_id_by_target "tasks_count" "$TIMEOUT_SEC" || true)"
if [[ -z "$SUCCESS_SID" ]]; then
  echo "FAIL: timeout waiting success schedule_id (target_tool_name=tasks_count)" >&2
  exit 1
fi
echo "  success schedule_id: $SUCCESS_SID"

echo "  waiting success tool_result (ok=true)..."
SUCCESS_RESULT_EVT="$(wait_for_jq_event \
  ".type==\"secretary.thinking\" and (.payload.schedule_id==\"${SUCCESS_SID}\") and (.payload.kind==\"tool_result\") and (.payload.target_tool_name==\"tasks_count\") and (.payload.ok==true)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$SUCCESS_RESULT_EVT" ]]; then
  echo "FAIL: timeout waiting success tool_result event" >&2
  exit 1
fi
echo "  got success tool_result event."

echo "  waiting success immediate secretary.message..."
SUCCESS_MSG_EVT="$(wait_for_jq_event \
  ".type==\"secretary.message\" and (.payload.schedule_id==\"${SUCCESS_SID}\") and ((.payload.content // \"\")|length>0)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$SUCCESS_MSG_EVT" ]]; then
  echo "FAIL: timeout waiting success secretary.message event" >&2
  exit 1
fi
echo "  got success secretary.message event."

echo "[5/7] create FAILURE scheduler (task_new_submit missing required fields)"
FAIL_PROMPT="$(cat <<EOF
请调用 scheduler_create 创建一个调度任务，参数如下：
- tool_name: task_new_submit
- tool_fields_json: {}
- interval_sec: ${INTERVAL_SEC}
- ttl_sec: ${TTL_SEC}
- allow_write: true
创建后请简单确认。
EOF
)"
post_secretary "$FAIL_PROMPT" | jq -r '.reply // ""' | sed 's/^/  secretary> /'

echo "  waiting failure schedule_id..."
FAIL_SID="$(wait_for_schedule_id_by_target "task_new_submit" "$TIMEOUT_SEC" || true)"
if [[ -z "$FAIL_SID" ]]; then
  echo "FAIL: timeout waiting failure schedule_id (target_tool_name=task_new_submit)" >&2
  exit 1
fi
echo "  failure schedule_id: $FAIL_SID"

echo "  waiting failure tool_result (ok=false)..."
FAIL_RESULT_EVT="$(wait_for_jq_event \
  ".type==\"secretary.thinking\" and (.payload.schedule_id==\"${FAIL_SID}\") and (.payload.kind==\"tool_result\") and (.payload.target_tool_name==\"task_new_submit\") and (.payload.ok==false)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$FAIL_RESULT_EVT" ]]; then
  echo "FAIL: timeout waiting failure tool_result event" >&2
  exit 1
fi
echo "  got failure tool_result event."

echo "  waiting failure immediate secretary.message..."
FAIL_MSG_EVT="$(wait_for_jq_event \
  ".type==\"secretary.message\" and (.payload.schedule_id==\"${FAIL_SID}\") and ((.payload.content // \"\")|length>0)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$FAIL_MSG_EVT" ]]; then
  echo "FAIL: timeout waiting failure secretary.message event" >&2
  exit 1
fi
echo "  got failure secretary.message event."

echo "[6/7] evidence snapshot"
echo "  success thinking: $(echo "$SUCCESS_RESULT_EVT" | jq -c '.')"
echo "  success message : $(echo "$SUCCESS_MSG_EVT" | jq -c '.payload')"
echo "  failure thinking: $(echo "$FAIL_RESULT_EVT" | jq -c '.')"
echo "  failure message : $(echo "$FAIL_MSG_EVT" | jq -c '.payload')"

echo "[7/7] PASS - validated: create scheduler -> auto polling -> success/failure immediate secretary report (SSE)"
echo "note: schedules use ttl_sec=${TTL_SEC}, they will auto-expire shortly."
