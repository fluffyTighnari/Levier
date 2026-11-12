#!/usr/bin/env bash
# Middleware — one-command single-node MVP demo.
#
# Starts PostgreSQL-backed Middleware, the real Harness, and the Counter App;
# submits one run; waits for a terminal status; prints a redacted summary.
#
# Modes:
#   echo    no model call, proves MW + Harness + App transport
#   react   real DeepSeek model call + counter tools
#
# Requirements:
#   - PostgreSQL reachable via MIDDLEWARE_DATABASE_URL
#   - harness/.venv (see README)
#   - react mode: DEEPSEEK_API_KEY in .env
#
# Usage:
#   make mvp-demo
#   make mvp-demo-react
#   MVP_DEMO_MODE=react scripts/mvp_demo.sh
#   MVP_DEMO_OUT=/tmp/mvp-demo scripts/mvp_demo.sh
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

MODE="${1:-${MVP_DEMO_MODE:-echo}}"
case "$MODE" in
  echo|react) ;;
  *) echo "mvp-demo: mode must be echo or react" >&2; exit 2 ;;
esac

MIDDLEWARE_BIN="${MIDDLEWARE_BIN:-bin/middleware}"
MIDDLEWARE_CONFIG="${MIDDLEWARE_CONFIG:-config.example.yaml}"
HARNESS_CONFIG="${HARNESS_CONFIG:-harness/config.example.yaml}"
HARNESS_PYTHON="${HARNESS_PYTHON:-harness/.venv/bin/python}"
MW_URL="${MW_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${MW_ADMIN_TOKEN:-admin-change-me}"
DATABASE_URL="${MIDDLEWARE_DATABASE_URL:-}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${MVP_DEMO_OUT:-data/mvp-demo/$STAMP-$MODE}"
CREDENTIALS="$OUT/credentials.json"
PROMPT_ECHO="hello mvp"
PROMPT_REACT="Add 2, then add 3, then read the counter and tell me the final value. Use the counter tools; do not calculate it yourself."
RUN_TIMEOUT_S="${MVP_DEMO_RUN_TIMEOUT:-180}"

fail() {
  echo "mvp-demo: $*" >&2
  exit 1
}

if [ -z "$DATABASE_URL" ]; then
  fail "MIDDLEWARE_DATABASE_URL is required (source .env.local or start PostgreSQL)"
fi
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
[ -x "$HARNESS_PYTHON" ] || fail "harness Python not found: $HARNESS_PYTHON (cd harness && uv sync --locked)"

mkdir -p "$OUT"
chmod 700 "$OUT"
MW_PID=""
HARNESS_PID=""
SERVE_PID=""

cleanup() {
  for pid in "$SERVE_PID" "$HARNESS_PID" "$MW_PID"; do
    if [ -n "$pid" ]; then
      kill -TERM "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -f "$CREDENTIALS"
}
trap cleanup EXIT

if curl -fsS "$MW_URL/healthz" >/dev/null 2>&1; then
  fail "$MW_URL is already serving; stop it before running the demo"
fi

echo "== build =="
make build >/dev/null

echo "== migrate =="
go run ./cmd/migrate -config "$MIDDLEWARE_CONFIG" >"$OUT/migrate.log" 2>&1 \
  || fail "migration failed; is PostgreSQL running? see $OUT/migrate.log"

echo "== start Middleware =="
"$MIDDLEWARE_BIN" -config "$MIDDLEWARE_CONFIG" >"$OUT/middleware.log" 2>&1 &
MW_PID=$!
ready=""
for _ in $(seq 1 100); do
  if curl -fsS "$MW_URL/healthz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.1
done
[ -n "$ready" ] || fail "Middleware did not become healthy; see $OUT/middleware.log"

echo "== start Harness =="
PYTHONPATH="agent_core/src:harness/src/gen:harness/src" "$HARNESS_PYTHON" -m middleware_harness \
  --config "$HARNESS_CONFIG" >"$OUT/harness.log" 2>&1 &
HARNESS_PID=$!
sleep 2

echo "== bootstrap App =="
MW_URL="$MW_URL" MW_ADMIN_TOKEN="$ADMIN_TOKEN" MW_APP_NAME="mvp-demo-$MODE-$STAMP" \
  python3 examples/counter_agent/bootstrap_app.py --credentials-out "$CREDENTIALS" \
  >"$OUT/bootstrap.json" 2>"$OUT/bootstrap.err" || fail "bootstrap failed; see $OUT/bootstrap.err"

echo "== start App =="
python3 examples/counter_agent/serve.py \
  --credentials "$CREDENTIALS" \
  --mw-url "$MW_URL" \
  --timeout 300 \
  >"$OUT/serve.log" 2>"$OUT/serve.err" &
SERVE_PID=$!
serving=""
for _ in $(seq 1 100); do
  if grep -q '"serving": true' "$OUT/serve.log" 2>/dev/null; then
    serving=1
    break
  fi
  sleep 0.1
done
[ -n "$serving" ] || fail "App did not start serving; see $OUT/serve.log"

echo "== submit $MODE run =="
if [ "$MODE" = "react" ]; then
  if [ -z "${DEEPSEEK_API_KEY:-}" ]; then
    fail "react mode requires DEEPSEEK_API_KEY"
  fi
  python3 examples/counter_agent/submit_run.py \
    --credentials "$CREDENTIALS" \
    --mw-url "$MW_URL" \
    --strategy react \
    --prompt "$PROMPT_REACT" \
    --provider deepseek \
    --model deepseek-flash \
    --max-steps 8 \
    --run-ms 120000 \
    --request-id "mvp-demo-$MODE-$STAMP" \
    >"$OUT/submit_run.json" 2>"$OUT/submit_run.err" || fail "run submission failed; see $OUT/submit_run.err"
else
  python3 examples/counter_agent/submit_run.py \
    --credentials "$CREDENTIALS" \
    --mw-url "$MW_URL" \
    --strategy echo \
    --prompt "$PROMPT_ECHO" \
    --run-ms 30000 \
    --request-id "mvp-demo-$MODE-$STAMP" \
    >"$OUT/submit_run.json" 2>"$OUT/submit_run.err" || fail "run submission failed; see $OUT/submit_run.err"
fi

RUN_ID="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["run_id"])' "$OUT/submit_run.json")"
echo "run_id=$RUN_ID"

echo "== wait for terminal state =="
python3 - "$MW_URL" "$CREDENTIALS" "$RUN_ID" "$RUN_TIMEOUT_S" "$MODE" "$OUT/summary.txt" <<'PY'
import json
import sys
import time
import urllib.error
import urllib.request

mw_url, credentials_path, run_id, timeout_s, mode, summary_path = sys.argv[1:7]
creds = json.load(open(credentials_path, encoding="utf-8"))
headers = {"Authorization": "Bearer " + creds["api_key"]}


def get(path):
    request = urllib.request.Request(mw_url + path, headers=headers)
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.loads(response.read().decode("utf-8"))


deadline = time.time() + float(timeout_s)
run = {}
while time.time() < deadline:
    run = get(f"/api/v1/runs/{run_id}")
    if run.get("status") in {"completed", "failed", "cancelled", "timeout"}:
        break
    time.sleep(1)
else:
    raise SystemExit("mvp-demo: run did not reach a terminal state")

status = run.get("status")
events = get(f"/api/v1/runs/{run_id}/events?limit=1000").get("events", [])
begin = sum(1 for event in events if event.get("event_type") == "TOOL_CALL_BEGIN")
end = sum(1 for event in events if event.get("event_type") == "TOOL_CALL_END")
errors = [event for event in events if event.get("event_type") == "ERROR"]
final = ""
for event in reversed(events):
    if event.get("event_type") != "MESSAGE_COMPLETED":
        continue
    content = (event.get("data") or {}).get("content") or ""
    if content:
        final = content
        break

summary = [
    f"mode: {mode}",
    f"run_id: {run_id}",
    f"status: {status}",
    f"attempt: {run.get('attempt')}",
    f"events: {len(events)}",
    f"tool_calls: begin={begin} end={end}",
    f"errors: {len(errors)}",
    f"final_message: {final[:500]}",
]
with open(summary_path, "w", encoding="utf-8") as handle:
    handle.write("\n".join(summary) + "\n")

if status != "completed":
    raise SystemExit(f"mvp-demo: run ended as {status}")
if errors:
    raise SystemExit("mvp-demo: run emitted ERROR events")
if mode == "echo":
    expected = "hello mvp"
    if expected not in final:
        raise SystemExit(f"mvp-demo: final message missing {expected!r}")
else:
    if begin != 3 or end != 3:
        raise SystemExit(f"mvp-demo: tool calls begin={begin} end={end}, want 3/3")
    if "5" not in final:
        raise SystemExit("mvp-demo: final message does not contain the expected value 5")
print("\n".join(summary))
PY

echo
cat "$OUT/summary.txt"
echo
echo "mvp-demo: PASS (evidence: $OUT)"
