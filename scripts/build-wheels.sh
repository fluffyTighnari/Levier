#!/bin/sh
# Levier - build Python wheels for the App SDK and Runtime packages.
#
# Usage:
#   sh scripts/build-wheels.sh
#   PYTHON=python3 OUT_DIR=dist sh scripts/build-wheels.sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

if [ -n "${PYTHON:-}" ]; then
	py=$PYTHON
elif command -v python3 >/dev/null 2>&1; then
	py=python3
elif command -v python >/dev/null 2>&1; then
	py=python
else
	echo "python3 or python is required to build wheels" >&2
	exit 2
fi

OUT_DIR=${OUT_DIR:-dist}
case "$OUT_DIR" in
	"" | "/" | "." | "..")
		echo "invalid OUT_DIR: $OUT_DIR" >&2
		exit 2
		;;
	/* | [A-Za-z]:[\\/]*) ;;
	*) OUT_DIR="$ROOT/$OUT_DIR" ;;
esac
mkdir -p "$OUT_DIR"

"$py" -m pip --version >/dev/null 2>&1 || {
	echo "pip is required: $py -m pip" >&2
	exit 2
}

packages="
sdk/python
sdk/levier
execution/agent_core
execution/runtime_sdk
execution/echo_runtime
execution/react_runtime
execution/mcp_runtime
execution/langgraph_runtime
"

for package in $packages; do
	echo "building wheel: $package"
	"$py" -m pip wheel --no-deps --wheel-dir "$OUT_DIR" "$ROOT/$package"
done

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo "sha256sum or shasum is required" >&2
		exit 2
	fi
}

bundle="$OUT_DIR/levier-python-wheels.tar.gz"
rm -f "$bundle"
(
	cd "$OUT_DIR"
	tar -czf "$bundle" ./*.whl
)

if [ -f "$OUT_DIR/checksums.txt" ]; then
	for artifact in "$OUT_DIR"/*.whl "$bundle"; do
		[ -f "$artifact" ] || continue
		name=$(basename "$artifact")
		if grep -F "  $name" "$OUT_DIR/checksums.txt" >/dev/null 2>&1; then
			continue
		fi
		printf '%s  %s\n' "$(sha256_file "$artifact")" "$name" >>"$OUT_DIR/checksums.txt"
	done
fi

echo
echo "python wheels: $OUT_DIR"
ls -1 "$OUT_DIR"/*.whl 2>/dev/null || true
echo "bundle:        $bundle"
