#!/usr/bin/env bash
# Levier — structure guard wrapper.
set -euo pipefail

cd "$(dirname "$0")/../.."

PYTHON=${PYTHON:-python3}
if ! command -v "$PYTHON" >/dev/null 2>&1; then
  PYTHON=python
fi

exec "$PYTHON" tests/checks/check_structure.py
