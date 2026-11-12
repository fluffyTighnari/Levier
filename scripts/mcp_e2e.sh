#!/usr/bin/env bash
# Middleware — MCP end-to-end regression (catalog → TaskAssign → runtime).
#
# Starts Middleware, the MCP runtime, the Harness and an App, registers a
# stdio MCP server with a ${secret:...} placeholder, and runs one real tool
# call through the model.
#
# Requirements:
#   - PostgreSQL reachable via MIDDLEWARE_DATABASE_URL
#   - DEEPSEEK_API_KEY in the environment
#   - bin/middleware built, harness/.venv and mcp_runtime/.venv present
set -euo pipefail

cd "$(dirname "$0")/.."

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

MIDDLEWARE_BIN="${MIDDLEWARE_BIN:-bin/middleware}"
MIDDLEWARE_CONFIG="${MIDDLEWARE_CONFIG:-config.example.yaml}"
HARNESS_CONFIG="${HARNESS_CONFIG:-harness/config.example.yaml}"
HARNESS_PYTHON="${HARNESS_PYTHON:-harness/.venv/bin/python}"
RUNTIME_PYTHON="${RUNTIME_PYTHON:-mcp_runtime/.venv/bin/python}"
MCP_SOCKET="${MCP_SOCKET:-/tmp/middleware-mcp.sock}"
MW_URL="${MW_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${MW_ADMIN_TOKEN:-admin-change-me}"
DATABASE_URL="${MIDDLEWARE_DATABASE_URL:-}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${MCP_E2E_OUT:-docs/reports/artifacts/mcp/M6/run-$STAMP}"
SECRET_VALUE="mcp-e2e-secret"
SERVER_NAME="e2e-echo"
TOOL_NAME="mcp__e2e_echo__echo_token"
FIXTURE="mcp_runtime/tests/secret_echo_server.py"

fail() {
  echo "mcp-e2e: $*" >&2
  exit 1
}

[ -n "$DATABASE_URL" ] || fail "MIDDLEWARE_DATABASE_URL is required"
command -v psql >/dev/null 2>&1 || fail "psql is required"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
[ -x "$HARNESS_PYTHON" ] || fail "harness Python not found: $HARNESS_PYTHON"
[ -x "$RUNTIME_PYTHON" ] || fail "mcp_runtime Python not found: $RUNTIME_PYTHON"
[ -f "$FIXTURE" ] || fail "MCP fixture not found: $FIXTURE"
if [ ! -x "$MIDDLEWARE_BIN" ]; then
  make build
fi

mkdir -p "$OUT"
chmod 700 "$OUT"
CREDENTIALS="$OUT/credentials.json"
MW_PID=""
HARNESS_PID=""
RUNTIME_PID=""

cleanup() {
  for pid in "$HARNESS_PID" "$RUNTIME_PID" "$MW_PID"; do
    if [ -n "$pid" ]; then
      kill -TERM "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -f "$CREDENTIALS" "$MCP_SOCKET"
}
trap cleanup EXIT

psql_run() {
  psql "$DATABASE_URL" -At -v ON_ERROR_STOP=1 -c "$1"
}

echo "== preflight =="
go run ./cmd/migrate -config "$MIDDLEWARE_CONFIG" >"$OUT/migrate.log" 2>&1 \
  || fail "migration failed; is PostgreSQL running?"
if curl -fsS "$MW_URL/healthz" >/dev/null 2>&1; then
  fail "$MW_URL is already serving; stop it first"
fi
rm -f "$MCP_SOCKET"

echo "== start Middleware =="
"$MIDDLEWARE_BIN" -config "$MIDDLEWARE_CONFIG" >"$OUT/middleware.log" 2>&1 &
MW_PID=$!
for _ in $(seq 1 100); do
  curl -fsS "$MW_URL/healthz" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fsS "$MW_URL/healthz" >/dev/null 2>&1 || fail "Middleware did not become healthy"

echo "== start MCP runtime =="
MIDDLEWARE_SECRET_E2E_TOKEN="$SECRET_VALUE" PYTHONPATH=mcp_runtime/src \
  "$RUNTIME_PYTHON" -m mcp_runtime --socket "$MCP_SOCKET" --log-level info \
  >"$OUT/runtime.log" 2>&1 &
RUNTIME_PID=$!
for _ in $(seq 1 100); do
  [ -S "$MCP_SOCKET" ] && break
  sleep 0.1
done
[ -S "$MCP_SOCKET" ] || fail "MCP runtime did not listen"

echo "== start Harness =="
PYTHONPATH="harness/src/gen:harness/src" "$HARNESS_PYTHON" -m middleware_harness \
  --config "$HARNESS_CONFIG" >"$OUT/harness.log" 2>&1 &
HARNESS_PID=$!
sleep 2

echo "== register catalog and app =="
python3 - "$RUNTIME_PYTHON" "$FIXTURE" >"$OUT/catalog_request.json" <<'PY'
import json, sys
runtime_python, fixture = sys.argv[1], sys.argv[2]
print(json.dumps({
    "name": "e2e-echo",
    "transport": "stdio",
    "config": {
        "command": runtime_python,
        "args": [fixture],
        "env": {"PROBE_TOKEN": "${secret:e2e_token}"},
    },
    "allowed_tools": ["echo_token"],
}, separators=(",", ":")))
PY
curl -fsS -X POST "$MW_URL/admin/mcp" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary "@$OUT/catalog_request.json" >"$OUT/catalog.json" || fail "catalog registration failed"

MW_URL="$MW_URL" MW_ADMIN_TOKEN="$ADMIN_TOKEN" MW_APP_NAME="mcp-e2e-$STAMP" \
  python3 examples/counter_agent/bootstrap_app.py --credentials-out "$CREDENTIALS" \
  >"$OUT/bootstrap.json" 2>"$OUT/bootstrap.err" || fail "bootstrap failed"

APP_NAME="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["app_name"])' "$CREDENTIALS")"
APP_KEY="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["api_key"])' "$CREDENTIALS")"
python3 - "$APP_NAME" >"$OUT/app_request.json" <<'PY'
import json, sys
print(json.dumps({"name": sys.argv[1], "mcp_refs": ["e2e-echo"]}, separators=(",", ":")))
PY
curl -fsS -X POST "$MW_URL/api/v1/apps" \
  -H "Authorization: Bearer $APP_KEY" \
  -H "Content-Type: application/json" \
  --data-binary "@$OUT/app_request.json" >"$OUT/app.json" || fail "app mcp_refs update failed"

echo "== run =="
python3 examples/counter_agent/submit_run.py \
  --credentials "$CREDENTIALS" \
  --mw-url "$MW_URL" \
  --strategy react \
  --prompt "Call the tool named $TOOL_NAME exactly once, then report the exact string it returns." \
  --provider deepseek \
  --model deepseek-flash \
  --max-steps 3 \
  --run-ms 120000 \
  >"$OUT/submit_run.json" 2>"$OUT/submit_run.err" || fail "run submission failed"
RUN_ID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["run_id"])' "$OUT/submit_run.json")"
echo "run_id=$RUN_ID"

STATUS=""
for _ in $(seq 1 120); do
  STATUS="$(psql_run "SELECT status FROM runs WHERE run_id='$RUN_ID';" | head -1)"
  case "$STATUS" in
    completed|failed|cancelled|timeout) break ;;
  esac
  sleep 1
done
[ "$STATUS" = "completed" ] || fail "run ended as $STATUS"

echo "== validate =="
END_OK="$(psql_run "SELECT count(*) FROM run_events WHERE run_id='$RUN_ID' AND type='TOOL_CALL_END' AND payload->>'tool_name'='$TOOL_NAME' AND payload->>'ok'='true';")"
RESULT_HIT="$(psql_run "SELECT count(*) FROM run_events WHERE run_id='$RUN_ID' AND type='TOOL_CALL_END' AND payload->>'result_json' LIKE '%$SECRET_VALUE%';")"
GATEWAY_ROWS="$(psql_run "SELECT count(*) FROM tool_invocations WHERE run_id='$RUN_ID';")"
ERRORS="$(psql_run "SELECT count(*) FROM run_events WHERE run_id='$RUN_ID' AND type='ERROR';")"
FINAL_HIT="$(psql_run "SELECT count(*) FROM run_events WHERE run_id='$RUN_ID' AND type='MESSAGE_COMPLETED' AND payload->>'content' LIKE '%$SECRET_VALUE%';")"

[ "$END_OK" = "1" ] || fail "MCP TOOL_CALL_END ok count = $END_OK, want 1"
[ "$RESULT_HIT" = "1" ] || fail "tool result did not contain the resolved secret"
[ "$GATEWAY_ROWS" = "0" ] || fail "MCP call wrote $GATEWAY_ROWS tool_invocations rows"
[ "$ERRORS" = "0" ] || fail "run emitted $ERRORS ERROR events"
[ "$FINAL_HIT" -ge 1 ] || fail "final message did not reference the tool result"
grep -q '"mcp_binding_count":1' "$OUT/harness.log" || fail "TaskAssign did not carry one MCP binding"
grep -q '"server_name":"e2e-echo"' "$OUT/harness.log" || fail "TaskAssign binding missing server name"
grep -q "mcp session opened" "$OUT/runtime.log" || fail "runtime did not open a session"

{
  echo "run_id: $RUN_ID"
  echo "status: $STATUS"
  echo "mcp_tool: $TOOL_NAME"
  echo "tool_call_end_ok: $END_OK"
  echo "result_contains_secret: $RESULT_HIT"
  echo "gateway_invocations: $GATEWAY_ROWS"
  echo "error_events: $ERRORS"
  echo "final_message_hit: $FINAL_HIT"
  echo "task_assign_binding: 1"
} >"$OUT/summary.txt"

echo
cat "$OUT/summary.txt"
echo
echo "mcp-e2e: PASS (evidence: $OUT)"
