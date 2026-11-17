#!/usr/bin/env bash
# Levier — file and function size baseline report.
#
# Advisory: files >450 are split candidates; functions >80 are review-only.
# Do not split a cohesive function just to satisfy the line limit.
set -euo pipefail

cd "$(dirname "$0")/../.."

FILE_LIMIT="${FILE_LIMIT:-450}"
FUNC_LIMIT="${FUNC_LIMIT:-80}"
PY_LIMIT="${PY_LIMIT:-300}"

echo "size report is advisory: split by responsibility, not by line count"
echo
echo "== Go files (non-test), top 20 =="
find internal cmd -name '*.go' -not -name '*_test.go' -exec wc -l {} + \
  | sort -n | tail -20

echo
echo "== Go files over ${FILE_LIMIT} lines =="
find internal cmd -name '*.go' -not -name '*_test.go' -exec wc -l {} + \
  | awk -v limit="$FILE_LIMIT" '$1 > limit && $2 != "total" {print}'

echo
echo "== Go functions over ${FUNC_LIMIT} lines =="
if [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi
go run ./tests/checks/size_functions "$FUNC_LIMIT"

echo
echo "== Python modules over ${PY_LIMIT} lines =="
find execution/harness/src execution/mcp_runtime/src -name '*.py' -exec wc -l {} + \
  | awk -v limit="$PY_LIMIT" '$1 > limit && $2 != "total" {print}' | sort -n

echo
echo "== counts =="
echo -n "Go non-test files: "; find internal cmd -name '*.go' -not -name '*_test.go' | wc -l | tr -d ' '
echo -n "Go lines: "; find internal cmd -name '*.go' -not -name '*_test.go' -exec cat {} + | wc -l | tr -d ' '
echo -n "Python source files: "; find execution/harness/src execution/mcp_runtime/src -name '*.py' | wc -l | tr -d ' '
echo -n "Python lines: "; find execution/harness/src execution/mcp_runtime/src -name '*.py' -exec cat {} + | wc -l | tr -d ' '
