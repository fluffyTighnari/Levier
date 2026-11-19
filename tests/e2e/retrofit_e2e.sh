#!/usr/bin/env bash
# Levier — retrofit one-click end-to-end regression.
#
# Starts Levier, the real React Runtime, and the Counter App; submits the frozen
# P0.1 prompt; then validates the run against the database and metrics.
#
# Requirements:
#   - PostgreSQL reachable via LEVIER_DATABASE_URL (see .env.local)
#   - DEEPSEEK_API_KEY in the environment (.env)
#   - bin/levier built, execution/react_runtime/.venv present
#
# Usage:
#   make retrofit-e2e
#   RETROFIT_E2E_OUT=/tmp/retrofit ./tests/e2e/retrofit_e2e.sh
set -euo pipefail

cd "$(dirname "$0")/../.."

if [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi
for env_file in .env.local .env; do
  if [ -f "$env_file" ]; then
    set -a
    # shellcheck disable=SC1090
    . "./$env_file"
    set +a
  fi
done

LEVIER_BIN="${LEVIER_BIN:-${MIDDLEWARE_BIN:-bin/levier}}"
LEVIER_CONFIG="${LEVIER_CONFIG:-${MIDDLEWARE_CONFIG:-levier.example.yaml}}"
REACT_RUNTIME_CONFIG="${REACT_RUNTIME_CONFIG:-execution/react_runtime/react.example.yaml}"
REACT_RUNTIME_PYTHON="${REACT_RUNTIME_PYTHON:-execution/react_runtime/.venv/bin/python}"
LEVIER_URL="${LEVIER_URL:-${MW_URL:-http://127.0.0.1:8080}}"
ADMIN_TOKEN="${LEVIER_ADMIN_TOKEN:-${MW_ADMIN_TOKEN:-admin-change-me}}"
DATABASE_URL="${LEVIER_DATABASE_URL:-${MIDDLEWARE_DATABASE_URL:-}}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${RETROFIT_E2E_OUT:-docs/reports/artifacts/retrofit/P10/run-$STAMP}"
PROMPT="Add 2, then add 3, then read the counter and tell me the final value. Use the counter tools; do not calculate it yourself."
TRACE="retrofit-e2e-$STAMP"

fail() {
  echo "retrofit-e2e: $*" >&2
  exit 1
}

if [ -z "$DATABASE_URL" ]; then
  fail "LEVIER_DATABASE_URL is required (source .env.local)"
fi
command -v psql >/dev/null 2>&1 || fail "psql is required for database assertions"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
[ -x "$REACT_RUNTIME_PYTHON" ] || fail "React Runtime Python not found: $REACT_RUNTIME_PYTHON"
if [ ! -x "$LEVIER_BIN" ]; then
  make build
fi

mkdir -p "$OUT"
chmod 700 "$OUT"
CREDENTIALS="$OUT/credentials.json"
MW_PID=""
REACT_RUNTIME_PID=""
SERVE_PID=""

cleanup() {
  for pid in "$SERVE_PID" "$REACT_RUNTIME_PID" "$MW_PID"; do
    if [ -n "$pid" ]; then
      kill -TERM "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -f "$CREDENTIALS"
}
trap cleanup EXIT

run_psql() {
  psql "$DATABASE_URL" -At -v ON_ERROR_STOP=1 -c "$1"
}

echo "== preflight =="
go run ./cmd/migrate -config "$LEVIER_CONFIG" >"$OUT/migrate.log" 2>&1 \
  || fail "migration failed; is PostgreSQL running? see $OUT/migrate.log"

if curl -fsS "$LEVIER_URL/healthz" >/dev/null 2>&1; then
  fail "$LEVIER_URL is already serving; stop it before running this script"
fi

echo "== start Levier =="
"$LEVIER_BIN" -config "$LEVIER_CONFIG" >"$OUT/middleware.log" 2>&1 &
MW_PID=$!
for _ in $(seq 1 100); do
  curl -fsS "$LEVIER_URL/healthz" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS "$LEVIER_URL/healthz" >/dev/null 2>&1 || fail "Levier did not become healthy"

echo "== start React Runtime =="
PYTHONPATH="execution/agent_core/src:execution/runtime_sdk/src:execution/react_runtime/src" "$REACT_RUNTIME_PYTHON" -m levier_react_runtime \
  --config "$REACT_RUNTIME_CONFIG" >"$OUT/react_runtime.log" 2>&1 &
REACT_RUNTIME_PID=$!
sleep 2

echo "== create App =="
"$LEVIER_BIN" app init \
  --name "retrofit-e2e-$STAMP" \
  --server "$LEVIER_URL" \
  --admin-token "$ADMIN_TOKEN" \
  --credentials-out "$CREDENTIALS" \
  >"$OUT/bootstrap.json" 2>"$OUT/bootstrap.err" || fail "app init failed"

APP_ID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["app_id"])' "$CREDENTIALS")"

echo "== start App =="
LEVIER_CREDENTIALS="$CREDENTIALS" PYTHONPATH="sdk/python/src" \
  python3 examples/counter_agent/app.py \
  >"$OUT/serve.log" 2>"$OUT/serve.err" &
SERVE_PID=$!
for _ in $(seq 1 150); do
  if curl -fsS "$LEVIER_URL/metrics" 2>/dev/null | grep -q "levier_ws_connections{app=\"$APP_ID\"} 1"; then
    break
  fi
  sleep 0.1
done
curl -fsS "$LEVIER_URL/metrics" 2>/dev/null | grep -q "levier_ws_connections{app=\"$APP_ID\"} 1" \
  || fail "App did not connect its WebSocket"

echo "== submit run =="
curl -fsS "$LEVIER_URL/metrics" >"$OUT/metrics_before.txt"
"$LEVIER_BIN" run submit \
  --credentials "$CREDENTIALS" \
  --server "$LEVIER_URL" \
  --runtime react1 \
  --prompt "$PROMPT" \
  --provider deepseek \
  --model deepseek-flash \
  --max-steps 8 \
  --request-id "$TRACE" \
  --json \
  >"$OUT/submit_run.json" 2>"$OUT/submit_run.err" || fail "run submission failed"
RUN_ID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["run_id"])' "$OUT/submit_run.json")"
echo "run_id=$RUN_ID trace=$TRACE"

echo "== wait for terminal state =="
STATUS=""
for _ in $(seq 1 120); do
  STATUS="$(run_psql "SELECT status FROM runs WHERE run_id='$RUN_ID';" | head -1)"
  case "$STATUS" in
    completed|failed|cancelled|timeout) break ;;
  esac
  sleep 1
done
[ -n "$STATUS" ] || fail "run row not found"
[ "$STATUS" = "completed" ] || fail "run ended as $STATUS"

echo "== validate database =="
BEGIN_COUNT="$(run_psql "SELECT count(*) FROM run_events WHERE run_id='$RUN_ID' AND type='TOOL_CALL_BEGIN';")"
END_COUNT="$(run_psql "SELECT count(*) FROM run_events WHERE run_id='$RUN_ID' AND type='TOOL_CALL_END';")"
INV_COUNT="$(run_psql "SELECT count(*) FROM tool_invocations WHERE run_id='$RUN_ID' AND status='completed';")"
FINAL_VALUE="$(run_psql "SELECT result_json FROM tool_invocations WHERE run_id='$RUN_ID' AND tool_name='counter_get' ORDER BY created_at DESC LIMIT 1;")"
SEQ_STATS="$(run_psql "SELECT count(*)||'/'||max(seq)||'/'||(max(seq)-min(seq)+1) FROM run_events WHERE run_id='$RUN_ID';")"
TASK_STATS="$(run_psql "SELECT string_agg(task_type||'='||status, ',' ORDER BY task_type) FROM run_tasks WHERE run_id='$RUN_ID';")"
TRACE_ID="$(run_psql "SELECT trace_id FROM runs WHERE run_id='$RUN_ID';")"

[ "$BEGIN_COUNT" = "3" ] || fail "TOOL_CALL_BEGIN=$BEGIN_COUNT, want 3"
[ "$END_COUNT" = "3" ] || fail "TOOL_CALL_END=$END_COUNT, want 3"
[ "$INV_COUNT" = "3" ] || fail "completed invocations=$INV_COUNT, want 3"
[ "$FINAL_VALUE" = '{"value": 5}' ] || fail "counter_get result=$FINAL_VALUE, want {\"value\": 5}"
[ "$TRACE_ID" = "$TRACE" ] || fail "trace_id=$TRACE_ID, want $TRACE"
IFS='/' read -r EVENT_COUNT MAX_SEQ EXPECTED_SEQ <<<"$SEQ_STATS"
[ "$EVENT_COUNT" = "$EXPECTED_SEQ" ] && [ "$MAX_SEQ" = "$EXPECTED_SEQ" ] \
  || fail "event seq gap: $SEQ_STATS"

echo "== validate metrics =="
curl -fsS "$LEVIER_URL/metrics" >"$OUT/metrics_after.txt"
grep -q '^levier_runs_total{runtime="normal",status="completed"} 1$' "$OUT/metrics_after.txt" \
  || fail "levier_runs_total{completed} != 1"
grep -q '^levier_tool_invocations_total{scope="local",status="ok",tool="counter_add"} 2$' "$OUT/metrics_after.txt" \
  || fail "counter_add ok count != 2"
grep -q '^levier_tool_invocations_total{scope="local",status="ok",tool="counter_get"} 1$' "$OUT/metrics_after.txt" \
  || fail "counter_get ok count != 1"
grep -q '^levier_events_total{type="TOOL_CALL_BEGIN"} 3$' "$OUT/metrics_after.txt" \
  || fail "event metric TOOL_CALL_BEGIN != 3"

echo "== write evidence =="
{
  echo "run_id: $RUN_ID"
  echo "trace_id: $TRACE_ID"
  echo "status: $STATUS"
  echo "tool_calls: begin=$BEGIN_COUNT end=$END_COUNT invocations=$INV_COUNT"
  echo "final_value: $FINAL_VALUE"
  echo "events: $SEQ_STATS (count/max_seq/expected)"
  echo "tasks: $TASK_STATS"
  echo "metrics: runs_completed=1 counter_add_ok=2 counter_get_ok=1 tool_begin=3"
} >"$OUT/summary.txt"

echo
cat "$OUT/summary.txt"
echo
echo "retrofit-e2e: PASS (evidence: $OUT)"
