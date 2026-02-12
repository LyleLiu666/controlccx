#!/usr/bin/env bash
set -euo pipefail

# Manual end-to-end acceptance helper for scheduler callback visibility:
# 1) create success scheduler and failure scheduler through secretary (/api/secretary/messages)
# 2) verify real-time SSE events (/api/events): secretary.thinking + secretary.message
# 3) verify assistant reports are persisted in secretary history (/api/secretary/messages GET)
# 4) verify "immediate report" lag is within threshold
#
# Usage:
#   ./scripts/manual_secretary_scheduler_visibility_e2e.sh
#
# Optional env:
#   BASE_URL=http://127.0.0.1:5174
#   TOKEN=<instance-token>
#   TIMEOUT_SEC=90
#   INTERVAL_SEC=2
#   TTL_SEC=25
#   REPORT_MAX_LAG_SEC=8
#   CREATE_ATTEMPTS=3
#   CREATE_WAIT_SEC=20

BASE_URL="${BASE_URL:-http://127.0.0.1:5174}"
TOKEN="${TOKEN:-}"
TIMEOUT_SEC="${TIMEOUT_SEC:-90}"
INTERVAL_SEC="${INTERVAL_SEC:-2}"
TTL_SEC="${TTL_SEC:-25}"
REPORT_MAX_LAG_SEC="${REPORT_MAX_LAG_SEC:-8}"
CREATE_ATTEMPTS="${CREATE_ATTEMPTS:-3}"
CREATE_WAIT_SEC="${CREATE_WAIT_SEC:-20}"

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

validate_positive_int() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || (( value <= 0 )); then
    echo "$name must be a positive integer, got: $value" >&2
    exit 1
  fi
}

validate_positive_int "TIMEOUT_SEC" "$TIMEOUT_SEC"
validate_positive_int "INTERVAL_SEC" "$INTERVAL_SEC"
validate_positive_int "TTL_SEC" "$TTL_SEC"
validate_positive_int "REPORT_MAX_LAG_SEC" "$REPORT_MAX_LAG_SEC"
validate_positive_int "CREATE_ATTEMPTS" "$CREATE_ATTEMPTS"
validate_positive_int "CREATE_WAIT_SEC" "$CREATE_WAIT_SEC"

HDR=()
if [[ -n "$TOKEN" ]]; then
  HDR=(-H "X-ControlCCX-Token: $TOKEN")
fi

EVENTS_JSONL="$(mktemp -t ccx_scheduler_visibility_e2e_events.XXXXXX)"
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

get_secretary_history() {
  curl -sS --fail-with-body "${HDR[@]}" \
    "${BASE_URL}/api/secretary/messages?limit=200"
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

create_schedule_until_seen() {
  local title="$1"
  local target_tool="$2"
  local tool_fields_json="$3"
  local interval_sec="$4"
  local ttl_sec="$5"
  local allow_write="$6"

  local attempt reply sid prompt
  for attempt in $(seq 1 "$CREATE_ATTEMPTS"); do
    prompt="$(cat <<EOF
这是一个端到端测试，请你现在立刻且只做这一件事：调用 scheduler_create。
参数必须严格如下（字段名不要改）：
- tool_name: ${target_tool}
- tool_fields_json: ${tool_fields_json}
- interval_sec: ${interval_sec}
- ttl_sec: ${ttl_sec}
- allow_write: ${allow_write}
不要提问，不要解释，调用后只返回一句“已创建”确认。
EOF
)"
    echo "  ${title} attempt ${attempt}/${CREATE_ATTEMPTS}" >&2
    reply="$(post_secretary "$prompt" | jq -r '.reply // ""')"
    if [[ -n "$reply" ]]; then
      echo "$reply" | sed 's/^/  secretary> /' >&2
    else
      echo "  secretary> <empty reply>" >&2
    fi

    sid="$(wait_for_schedule_id_by_target "$target_tool" "$CREATE_WAIT_SEC" || true)"
    if [[ -n "$sid" ]]; then
      echo "$sid"
      return 0
    fi
  done
  return 1
}

wait_for_history_assistant_content() {
  local expected_content="$1"
  local timeout_sec="$2"

  local started deadline line
  started="$(date +%s)"
  deadline=$(( started + timeout_sec ))

  while (( "$(date +%s)" <= deadline )); do
    line="$(get_secretary_history \
      | jq -c --arg expected "$expected_content" '
          .messages[]?
          | select(.role=="assistant")
          | select((.content // "") == $expected)
        ' \
      | tail -n 1 || true)"
    if [[ -n "$line" ]]; then
      echo "$line"
      return 0
    fi
    sleep 1
  done
  return 1
}

calc_lag_seconds() {
  local schedule_id="$1"
  local target_tool="$2"
  local expected_ok="$3"
  jq -sr \
    --arg sid "$schedule_id" \
    --arg target "$target_tool" \
    --argjson expected_ok "$expected_ok" \
    '
    def e: (fromdateiso8601? // 0);
    (map(
      select(.type=="secretary.thinking")
      | select((.payload.schedule_id // "") == $sid)
      | select((.payload.kind // "") == "tool_result")
      | select((.payload.target_tool_name // "") == $target)
      | select((.payload.ok // false) == $expected_ok)
    ) | .[0].time) as $thinking_time
    | (map(
      select(.type=="secretary.message")
      | select((.payload.schedule_id // "") == $sid)
    ) | .[0].time) as $message_time
    | if ($thinking_time == null or $message_time == null) then
        ""
      else
        (($message_time | e) - ($thinking_time | e) | tostring)
      end
    ' "$EVENTS_JSONL"
}

assert_lag_within_threshold() {
  local lag="$1"
  local threshold="$2"
  if [[ -z "$lag" ]]; then
    echo "FAIL: could not compute report lag" >&2
    exit 1
  fi
  if ! awk -v lag="$lag" -v max="$threshold" 'BEGIN { exit !(lag >= 0 && lag <= max) }'; then
    echo "FAIL: report lag out of range, lag=${lag}s max=${threshold}s" >&2
    exit 1
  fi
}

echo "[0/9] quick server check"
curl -sS --fail-with-body "${HDR[@]}" "${BASE_URL}/api/system" >/dev/null

echo "[1/9] clear old secretary history"
curl -sS --fail-with-body "${HDR[@]}" \
  -X POST \
  -H "Content-Type: application/json" \
  "${BASE_URL}/api/secretary/clear" \
  -d '{}' >/dev/null

echo "[2/9] start SSE capture: ${BASE_URL}/api/events"
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

echo "[3/9] prime report format (must include [schedule:<id>])"
post_secretary "从现在开始，所有定时调度回调都请你立刻汇报一行中文，格式必须包含 [schedule:<schedule_id>]，成功写“成功”，失败写“失败”。" \
  | jq -r '.reply // ""' \
  | sed 's/^/  secretary> /'

echo "[4/9] create SUCCESS scheduler (tasks_count)"
SUCCESS_SID="$(create_schedule_until_seen "success create" "tasks_count" "{}" "${INTERVAL_SEC}" "${TTL_SEC}" "false" || true)"
if [[ -z "$SUCCESS_SID" ]]; then
  echo "FAIL: timeout creating success schedule_id (target_tool_name=tasks_count)" >&2
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

echo "  waiting success secretary.message..."
SUCCESS_MSG_EVT="$(wait_for_jq_event \
  ".type==\"secretary.message\" and (.payload.schedule_id==\"${SUCCESS_SID}\") and ((.payload.content // \"\")|length>0)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$SUCCESS_MSG_EVT" ]]; then
  echo "FAIL: timeout waiting success secretary.message event" >&2
  exit 1
fi
SUCCESS_MSG_CONTENT="$(echo "$SUCCESS_MSG_EVT" | jq -r '.payload.content // ""')"

echo "  waiting success assistant message in /api/secretary/messages..."
SUCCESS_HISTORY_MSG="$(wait_for_history_assistant_content "$SUCCESS_MSG_CONTENT" "$TIMEOUT_SEC" || true)"
if [[ -z "$SUCCESS_HISTORY_MSG" ]]; then
  echo "FAIL: timeout waiting success assistant message in secretary history" >&2
  exit 1
fi

echo "[5/9] create FAILURE scheduler (task_log_get with non-existent task_id)"
FAIL_SID="$(create_schedule_until_seen "failure create" "task_log_get" '{"task_id":"__e2e_missing_task__","log_id":"1"}' "${INTERVAL_SEC}" "${TTL_SEC}" "false" || true)"
if [[ -z "$FAIL_SID" ]]; then
  echo "FAIL: timeout creating failure schedule_id (target_tool_name=task_log_get)" >&2
  exit 1
fi
echo "  failure schedule_id: $FAIL_SID"

echo "  waiting failure tool_result (ok=false)..."
FAIL_RESULT_EVT="$(wait_for_jq_event \
  ".type==\"secretary.thinking\" and (.payload.schedule_id==\"${FAIL_SID}\") and (.payload.kind==\"tool_result\") and (.payload.target_tool_name==\"task_log_get\") and (.payload.ok==false)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$FAIL_RESULT_EVT" ]]; then
  echo "FAIL: timeout waiting failure tool_result event" >&2
  exit 1
fi

echo "  waiting failure secretary.message..."
FAIL_MSG_EVT="$(wait_for_jq_event \
  ".type==\"secretary.message\" and (.payload.schedule_id==\"${FAIL_SID}\") and ((.payload.content // \"\")|length>0)" \
  "$TIMEOUT_SEC" || true)"
if [[ -z "$FAIL_MSG_EVT" ]]; then
  echo "FAIL: timeout waiting failure secretary.message event" >&2
  exit 1
fi
FAIL_MSG_CONTENT="$(echo "$FAIL_MSG_EVT" | jq -r '.payload.content // ""')"

echo "  waiting failure assistant message in /api/secretary/messages..."
FAIL_HISTORY_MSG="$(wait_for_history_assistant_content "$FAIL_MSG_CONTENT" "$TIMEOUT_SEC" || true)"
if [[ -z "$FAIL_HISTORY_MSG" ]]; then
  echo "FAIL: timeout waiting failure assistant message in secretary history" >&2
  exit 1
fi

echo "[6/9] verify immediate report lag threshold"
SUCCESS_LAG_SEC="$(calc_lag_seconds "$SUCCESS_SID" "tasks_count" "true")"
FAIL_LAG_SEC="$(calc_lag_seconds "$FAIL_SID" "task_log_get" "false")"
assert_lag_within_threshold "$SUCCESS_LAG_SEC" "$REPORT_MAX_LAG_SEC"
assert_lag_within_threshold "$FAIL_LAG_SEC" "$REPORT_MAX_LAG_SEC"

echo "[7/9] evidence snapshot"
echo "  success thinking : $(echo "$SUCCESS_RESULT_EVT" | jq -c '.')"
echo "  success message  : $(echo "$SUCCESS_MSG_EVT" | jq -c '.payload')"
echo "  success history  : $(echo "$SUCCESS_HISTORY_MSG" | jq -c '{id,time,role,content}')"
echo "  success lag(sec) : $SUCCESS_LAG_SEC"
echo "  failure thinking : $(echo "$FAIL_RESULT_EVT" | jq -c '.')"
echo "  failure message  : $(echo "$FAIL_MSG_EVT" | jq -c '.payload')"
echo "  failure history  : $(echo "$FAIL_HISTORY_MSG" | jq -c '{id,time,role,content}')"
echo "  failure lag(sec) : $FAIL_LAG_SEC"

echo "[8/9] PASS - validated end-to-end chain"
echo "  create scheduler -> auto polling -> immediate secretary report"
echo "  realtime SSE and chat history are consistent for success/failure"
echo "[9/9] note: schedules use ttl_sec=${TTL_SEC}, they will auto-expire shortly."
