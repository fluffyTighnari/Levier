#!/usr/bin/env bash
# Middleware — package dependency guard.
#
# Enforces the P8R layering rules. Run after moving files or adding packages.
set -euo pipefail

cd "$(dirname "$0")/../.."

if [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
go list -f '{{.ImportPath}}|{{join .Imports " "}}' ./internal/... >"$TMP"

python3 - "$TMP" <<'PY'
import sys

MODULE = "github.com/fluffyTighnari/middleware/"

# package -> prefixes it must not import
DENY = {
    "internal/runstore": [
        "internal/control", "internal/gateway", "internal/httpapi",
        "internal/connection", "internal/register", "internal/workflow",
    ],
    "internal/register": [
        "internal/control", "internal/gateway", "internal/httpapi",
        "internal/connection",
    ],
    "internal/connection": [
        "internal/control", "internal/gateway", "internal/httpapi",
        "internal/register", "internal/runstore",
    ],
    "internal/workflow": [
        "internal/control", "internal/gateway", "internal/httpapi",
        "internal/connection", "internal/register",
    ],
    "internal/gateway": [
        "internal/control", "internal/httpapi",
    ],
    "internal/httpapi": [
        "internal/control",
    ],
    "internal/control": [
        "internal/httpapi",
    ],
    "internal/config": [
        "internal/",
    ],
    "internal/contract": [
        "internal/",
    ],
}

packages = []
with open(sys.argv[1], encoding="utf-8") as handle:
    lines = handle.read().splitlines()
for line in lines:
    line = line.strip()
    if not line:
        continue
    path, _, imports = line.partition("|")
    path = path.removeprefix(MODULE)
    packages.append((path, [i.removeprefix(MODULE) for i in imports.split()]))

violations = []
checked = 0
for path, imports in packages:
    denied = DENY.get(path)
    if not denied:
        continue
    checked += 1
    for imp in imports:
        for prefix in denied:
            if imp == prefix or imp.startswith(prefix + "/"):
                violations.append(f"{path} imports {imp} (denied prefix {prefix})")
                break

if violations:
    print("dependency check: FAIL")
    for item in violations:
        print("  " + item)
    sys.exit(1)

print(f"dependency check: OK ({checked} packages with rules, {len(packages)} packages scanned)")
PY
