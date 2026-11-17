#!/bin/sh
# Levier — Counter Agent container entrypoint.
set -eu

LEVIER_URL="${LEVIER_URL:-${MW_URL:-http://levier:8080}}"
CREDENTIALS="${COUNTER_APP_CREDENTIALS:-/data/credentials.json}"

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

if [ ! -f "$CREDENTIALS" ]; then
  python3 /app/examples/counter_agent/bootstrap_app.py \
    --credentials-out "$CREDENTIALS"
fi

exec python3 /app/examples/counter_agent/serve.py \
  --credentials "$CREDENTIALS" \
  --mw-url "$LEVIER_URL" \
  --timeout 86400
