#!/usr/bin/env bash
# Middleware — P4-36 M1 end-to-end gate script.
# Copyright 2026 The Middleware Authors.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi
if [ -f .env.local ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env.local
  set +a
fi

MIDDLEWARE_BIN="${MIDDLEWARE_BIN:-bin/middleware}"
MIDDLEWARE_CONFIG="${MIDDLEWARE_CONFIG:-config.example.yaml}"
HARNESS_CONFIG="${HARNESS_CONFIG:-harness/config.example.yaml}"
HARNESS_PYTHON="${HARNESS_PYTHON:-harness/.venv/bin/python}"
MW_URL="${MW_URL:-http://127.0.0.1:8080}"
BEARER_TOKEN="${BEARER_TOKEN:-change-me}"
APP_ID="${APP_ID:-app-m1-e2e}"
ARTIFACT_DIR="${ARTIFACT_DIR:-docs/reports/artifacts/P4-36}"
PSQL="${PSQL:-psql}"

if [ -z "${MIDDLEWARE_DATABASE_URL:-}" ]; then
  echo "MIDDLEWARE_DATABASE_URL is required (source .env.local)" >&2
  exit 1
fi
if [ ! -x "$HARNESS_PYTHON" ]; then
  echo "harness Python not found: $HARNESS_PYTHON" >&2
  echo "run: cd harness && uv sync --locked" >&2
  exit 1
fi

mkdir -p "$ARTIFACT_DIR"
MW_LOG="$ARTIFACT_DIR/middleware.log"
HARNESS_LOG="$ARTIFACT_DIR/harness.log"
MW_PID=""
HARNESS_PID=""

cleanup() {
  if [ -n "$HARNESS_PID" ]; then
    kill -TERM "$HARNESS_PID" 2>/dev/null || true
    wait "$HARNESS_PID" 2>/dev/null || true
  fi
  if [ -n "$MW_PID" ]; then
    kill -TERM "$MW_PID" 2>/dev/null || true
    wait "$MW_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "== migrations ==" | tee "$ARTIFACT_DIR/summary.txt"
go run ./cmd/migrate -config "$MIDDLEWARE_CONFIG" 2>&1 | tee -a "$ARTIFACT_DIR/summary.txt"

"$MIDDLEWARE_BIN" -config "$MIDDLEWARE_CONFIG" >"$MW_LOG" 2>&1 &
MW_PID=$!

ready=""
for _ in $(seq 1 50); do
  if curl -fsS "$MW_URL/healthz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.2
done
if [ -z "$ready" ]; then
  echo "middleware did not become ready" >&2
  cat "$MW_LOG" >&2
  exit 1
fi

PYTHONPATH=harness/src/gen:harness/src "$HARNESS_PYTHON" \
  -m middleware_harness --config "$HARNESS_CONFIG" >"$HARNESS_LOG" 2>&1 &
HARNESS_PID=$!
sleep 1.5

create="$(curl -fsS -X POST "$MW_URL/api/v1/runs" \
  -H "Authorization: Bearer $BEARER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"app_id\":\"$APP_ID\",\"runtime\":\"normal\",\"strategy\":{\"name\":\"echo\"},\"input\":[{\"role\":\"user\",\"content\":\"hello m1 e2e\"}],\"model\":{\"provider\":\"anthropic\",\"model\":\"claude-test\"},\"timeouts\":{\"run_ms\":10000,\"tool_ms\":5000}}")"
printf '%s\n' "$create" >"$ARTIFACT_DIR/create.json"
echo "create: $create"
RUN_ID="$(printf '%s' "$create" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])')"
echo "run_id: $RUN_ID" | tee -a "$ARTIFACT_DIR/summary.txt"

state=""
for _ in $(seq 1 50); do
  run="$(curl -fsS "$MW_URL/api/v1/runs/$RUN_ID" -H "Authorization: Bearer $BEARER_TOKEN")"
  state="$(printf '%s' "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
  if [ "$state" = "completed" ] || [ "$state" = "failed" ] || [ "$state" = "cancelled" ] || [ "$state" = "timeout" ]; then
    break
  fi
  sleep 0.2
done
printf '%s\n' "$run" >"$ARTIFACT_DIR/run.json"
printf '%s\n' "$state" >"$ARTIFACT_DIR/status.txt"
echo "run status: $state" | tee -a "$ARTIFACT_DIR/summary.txt"

events="$(curl -fsS "$MW_URL/api/v1/runs/$RUN_ID/events" -H "Authorization: Bearer $BEARER_TOKEN")"
printf '%s\n' "$events" >"$ARTIFACT_DIR/events.json"
printf '%s' "$events" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
events = payload["events"]
assert [event["seq"] for event in events] == [1, 2, 3], events
assert [event["event_type"] for event in events] == [
    "RUN_STATUS", "MESSAGE_COMPLETED", "RUN_STATUS",
], events
assert events[0]["data"]["status"] == "running", events[0]
assert events[2]["data"]["status"] == "completed", events[2]
assert payload["next_attempt"] == 1 and payload["next_seq"] == 3, payload
'
echo "events: seq 1/2/3 valid" | tee -a "$ARTIFACT_DIR/summary.txt"

if [ "$state" != "completed" ]; then
  echo "run ended in state $state" >&2
  cat "$MW_LOG" >&2
  cat "$HARNESS_LOG" >&2
  exit 1
fi

if command -v "$PSQL" >/dev/null 2>&1; then
  "$PSQL" "$MIDDLEWARE_DATABASE_URL" -v ON_ERROR_STOP=1 -P pager=off \
    -c "SELECT task_id, run_id, task_type, status, attempt, max_attempts, lease_owner, lease_token, lease_expires_at, updated_at FROM run_tasks WHERE run_id = '$RUN_ID';" \
    >"$ARTIFACT_DIR/run_task.txt"
  "$PSQL" "$MIDDLEWARE_DATABASE_URL" -v ON_ERROR_STOP=1 -P pager=off \
    -c "SELECT attempt, seq, source, type, payload, ts_ms FROM run_events WHERE run_id = '$RUN_ID' ORDER BY attempt, seq;" \
    >"$ARTIFACT_DIR/run_events.txt"
  task_state="$("$PSQL" "$MIDDLEWARE_DATABASE_URL" -tAc "SELECT status FROM run_tasks WHERE run_id = '$RUN_ID' ORDER BY created_at DESC LIMIT 1")"
  if [ "$task_state" != "DONE" ]; then
    echo "workflow task state = $task_state, want DONE" >&2
    exit 1
  fi
  echo "workflow task: DONE" | tee -a "$ARTIFACT_DIR/summary.txt"
else
  echo "psql not found; skipped database snapshots" | tee -a "$ARTIFACT_DIR/summary.txt"
fi

echo "artifacts: $ARTIFACT_DIR" | tee -a "$ARTIFACT_DIR/summary.txt"
