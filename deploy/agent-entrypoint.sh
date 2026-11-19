#!/bin/sh
# Levier — Counter Agent test container entrypoint.
#
# Production agents belong to the user and are deployed by the user. This
# entrypoint only starts the test App from a credentials file created with the
# canonical CLI:
#
#   levier app init --name counter-agent \
#     --credentials-out data/agent/credentials.json
#
# It intentionally does not receive an Admin token and does not bootstrap App
# identities at runtime.
set -eu

LEVIER_URL="${LEVIER_APP_URL:-${LEVIER_URL:-${MW_URL:-http://levier:8080}}}"
CREDENTIALS="${LEVIER_APP_CREDENTIALS:-${LEVIER_CREDENTIALS:-/data/credentials.json}}"

python3 - "$LEVIER_URL" <<'PY'
import sys
import time
import urllib.request

url = sys.argv[1].rstrip("/") + "/readyz"
deadline = time.time() + 120
while time.time() < deadline:
    try:
        with urllib.request.urlopen(url, timeout=3) as response:
            if response.status == 200:
                sys.exit(0)
    except OSError:
        pass
    time.sleep(1)
raise SystemExit("agent: middleware readiness timed out")
PY

for _ in $(seq 1 120); do
  if [ -f "$CREDENTIALS" ]; then
    break
  fi
  sleep 1
done

if [ ! -f "$CREDENTIALS" ]; then
  echo "agent: credentials file not found: $CREDENTIALS" >&2
  echo "agent: create it with:" >&2
  echo "  levier app init --name counter-agent --server $LEVIER_URL --credentials-out $CREDENTIALS" >&2
  exit 1
fi

# Windows bind mounts do not preserve POSIX modes. The host credential is
# still created as 0600; normalize it inside the Linux test container.
chmod 600 "$CREDENTIALS" 2>/dev/null || true

export LEVIER_CREDENTIALS="$CREDENTIALS"
exec python3 /app/examples/counter_agent/app.py
