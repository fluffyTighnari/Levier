#!/usr/bin/env bash
# Middleware — file and function size baseline report.
#
# Reports the largest non-test Go files, long Go functions, and large Python
# modules. P8R.1 records the baseline; later steps compare against it.
set -euo pipefail

cd "$(dirname "$0")/.."

FILE_LIMIT="${FILE_LIMIT:-450}"
FUNC_LIMIT="${FUNC_LIMIT:-80}"
PY_LIMIT="${PY_LIMIT:-300}"

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
go run ./scripts/size_functions.go "$FUNC_LIMIT"

echo
echo "== Python modules over ${PY_LIMIT} lines =="
find harness/src mcp_runtime/src -name '*.py' -exec wc -l {} + \
  | awk -v limit="$PY_LIMIT" '$1 > limit && $2 != "total" {print}' | sort -n

echo
echo "== counts =="
echo -n "Go non-test files: "; find internal cmd -name '*.go' -not -name '*_test.go' | wc -l | tr -d ' '
echo -n "Go lines: "; find internal cmd -name '*.go' -not -name '*_test.go' -exec cat {} + | wc -l | tr -d ' '
echo -n "Python source files: "; find harness/src mcp_runtime/src -name '*.py' | wc -l | tr -d ' '
echo -n "Python lines: "; find harness/src mcp_runtime/src -name '*.py' -exec cat {} + | wc -l | tr -d ' '
