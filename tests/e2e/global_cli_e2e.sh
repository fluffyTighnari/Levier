#!/bin/sh
# Verify that the installed CLI can be used outside the repository.
#
# Requires a running Levier stack and LEVIER_ADMIN_TOKEN.
# Usage:
#   LEVIER_ADMIN_TOKEN=admin-change-me ./tests/e2e/global_cli_e2e.sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
SERVER=${LEVIER_E2E_SERVER:-http://127.0.0.1:8080}
ADMIN=${LEVIER_ADMIN_TOKEN:-}
RUNTIME=${LEVIER_E2E_RUNTIME:-echo1}

if [ -z "$ADMIN" ]; then
	printf 'skip: LEVIER_ADMIN_TOKEN is required for the global CLI end-to-end test\n' >&2
	exit 0
fi

WORK=$(mktemp -d "${TMPDIR:-/tmp}/levier-global-e2e.XXXXXX")
trap 'rm -rf "$WORK"' EXIT INT TERM

mkdir -p "$WORK/home" "$WORK/business"
BIN=${LEVIER_BIN:-$ROOT/bin/levier}
if [ ! -x "$BIN" ] && [ -x "$BIN.exe" ]; then
	BIN="$BIN.exe"
fi
if [ ! -x "$BIN" ]; then
	(cd "$ROOT" && go build -o "$BIN" ./cmd/levier)
fi

export LEVIER_HOME="$WORK/home"
export LEVIER_SERVER="$SERVER"
export LEVIER_ADMIN_TOKEN="$ADMIN"
unset LEVIER_CREDENTIALS LEVIER_APP_CREDENTIALS LEVIER_CONFIG LEVIER_STACK_FILE 2>/dev/null || true

cd "$WORK/business"
"$BIN" setup --server "$SERVER" --credentials "$WORK/home/credentials/demo.json" >/dev/null
"$BIN" setup --show --json >/dev/null
APP_NAME="global-e2e-$$"
"$BIN" app init --name "$APP_NAME" --json >/dev/null

APP_ID=$("$BIN" app show --json | sed -n 's/.*"app_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [ -z "$APP_ID" ]; then
	printf 'fail: app show did not return an app id\n' >&2
	exit 1
fi

OUTPUT=$("$BIN" run submit --runtime "$RUNTIME" --prompt "global cli e2e" --watch --timeout 30s)
printf '%s\n' "$OUTPUT"
printf '%s\n' "$OUTPUT" | grep -F 'final: global cli e2e' >/dev/null || {
	printf 'fail: run did not complete with the expected echo output\n' >&2
	exit 1
}

printf 'global cli e2e: PASS\n'
