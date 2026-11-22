#!/usr/bin/env bash
# Levier — structure guard wrapper.
set -euo pipefail

cd "$(dirname "$0")/../.."

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  tracked_ignored="$(git ls-files -ci --exclude-standard)"
  if [ -n "$tracked_ignored" ]; then
    echo "tracked files match .gitignore:" >&2
    printf '%s\n' "$tracked_ignored" >&2
    exit 1
  fi
fi

PYTHON=${PYTHON:-python3}
if ! command -v "$PYTHON" >/dev/null 2>&1; then
  PYTHON=python
fi

exec "$PYTHON" tests/checks/check_structure.py
