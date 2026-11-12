#!/usr/bin/env bash
# Middleware — P4-35 echo end-to-end smoke test.
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
APP_ID="${APP_ID:-app-e2e}"

if [ ! -x "$MIDDLEWARE_BIN" ]; then
  make build
fi
if [ ! -x "$HARNESS_PYTHON" ]; then
  echo "harness Python not found: $HARNESS_PYTHON" >&2
  echo "run: cd harness && uv sync --locked" >&2
  exit 1
fi

MW_LOG="$(mktemp -t mw-e2e.XXXXXX)"
HARNESS_LOG="$(mktemp -t harness-e2e.XXXXXX)"
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
  rm -f "$MW_LOG" "$HARNESS_LOG"
}
trap cleanup EXIT

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

PYTHONPATH=agent_core/src:harness/src/gen:harness/src "$HARNESS_PYTHON" \
  -m middleware_harness --config "$HARNESS_CONFIG" >"$HARNESS_LOG" 2>&1 &
HARNESS_PID=$!
sleep 1.5

create="$(curl -fsS -X POST "$MW_URL/api/v1/runs" \
  -H "Authorization: Bearer $BEARER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"app_id\":\"$APP_ID\",\"runtime\":\"normal\",\"strategy\":{\"name\":\"echo\"},\"input\":[{\"role\":\"user\",\"content\":\"hello e2e\"}],\"model\":{\"provider\":\"anthropic\",\"model\":\"claude-test\"},\"timeouts\":{\"run_ms\":10000,\"tool_ms\":5000}}")"
echo "create: $create"
RUN_ID="$(printf '%s' "$create" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])')"

state=""
for _ in $(seq 1 50); do
  run="$(curl -fsS "$MW_URL/api/v1/runs/$RUN_ID" -H "Authorization: Bearer $BEARER_TOKEN")"
  state="$(printf '%s' "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
  if [ "$state" = "completed" ]; then
    break
  fi
  if [ "$state" = "failed" ] || [ "$state" = "cancelled" ] || [ "$state" = "timeout" ]; then
    break
  fi
  sleep 0.2
done

events="$(curl -fsS "$MW_URL/api/v1/runs/$RUN_ID/events" -H "Authorization: Bearer $BEARER_TOKEN")"
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
if [ "$state" != "completed" ]; then
  echo "run ended in state $state" >&2
  cat "$MW_LOG" >&2
  cat "$HARNESS_LOG" >&2
  exit 1
fi

echo "run: $RUN_ID completed"
echo "events: $events"
