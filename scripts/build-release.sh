#!/bin/sh
# Levier — build release archives and checksums.
#
# Usage:
#   VERSION=v0.1.0 COMMIT=abc1234 BUILT=2026-11-17T00:00:00Z \
#     sh scripts/build-release.sh
#
# Environment:
#   VERSION   release version; defaults to git describe
#   COMMIT    short commit; defaults to git rev-parse --short HEAD
#   BUILT     UTC build timestamp; defaults to current time
#   OUT_DIR   output directory; defaults to dist
set -eu

MODULE="github.com/fluffyTighnari/levier"
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}
BUILT=${BUILT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
OUT_DIR=${OUT_DIR:-dist}

case "$OUT_DIR" in
	"" | "/" | "." | "..")
		echo "invalid OUT_DIR: $OUT_DIR" >&2
		exit 2
		;;
	/*) ;;
	*) OUT_DIR="$ROOT/$OUT_DIR" ;;
esac

if [ -z "$VERSION" ] || [ -z "$COMMIT" ] || [ -z "$BUILT" ]; then
	echo "VERSION, COMMIT and BUILT must not be empty" >&2
	exit 2
fi

case "$VERSION" in
	*/*)
		echo "VERSION must not contain '/': $VERSION" >&2
		exit 2
		;;
esac

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/levier-release.XXXXXX")
trap 'rm -rf "$WORK"' EXIT INT TERM

LDFLAGS="-X ${MODULE}/internal/platform/version.Version=${VERSION} -X ${MODULE}/internal/platform/version.Commit=${COMMIT} -X ${MODULE}/internal/platform/version.Built=${BUILT}"

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

zip_dir() {
	src=$1
	dst=$2
	if command -v zip >/dev/null 2>&1; then
		(cd "$src" && zip -q -r "$dst" .)
	elif command -v python3 >/dev/null 2>&1; then
		python3 - "$dst" "$src" <<'PY'
import os
import sys
import zipfile

dst, src = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as archive:
    for root, _, files in os.walk(src):
        for name in sorted(files):
            path = os.path.join(root, name)
            archive.write(path, os.path.relpath(path, src))
PY
	else
		echo "zip or python3 is required for Windows archives" >&2
		exit 2
	fi
}

tar_dir() {
	src=$1
	dst=$2
	bin=$3
	if command -v python3 >/dev/null 2>&1; then
		python3 - "$dst" "$src" "$bin" <<'PY'
import os
import sys
import tarfile

dst, src, bin_name = sys.argv[1], sys.argv[2], sys.argv[3]
path = os.path.join(src, bin_name)
with tarfile.open(dst, "w:gz") as archive:
    info = archive.gettarinfo(path, arcname=bin_name)
    info.mode = 0o755
    info.uid = 0
    info.gid = 0
    info.uname = ""
    info.gname = ""
    with open(path, "rb") as handle:
        archive.addfile(info, handle)
PY
	elif tar --help 2>&1 | grep -q -- '--mode'; then
		tar --mode='u+rwx,go+rx' -C "$src" -czf "$dst" "$bin"
	else
		tar -C "$src" -czf "$dst" "$bin"
	fi
}

build_target() {
	goos=$1
	goarch=$2
	name="levier_${VERSION}_${goos}_${goarch}"
	stage="$WORK/$name"
	mkdir -p "$stage"

	bin="levier"
	if [ "$goos" = "windows" ]; then
		bin="levier.exe"
	fi

	echo "building ${goos}/${goarch}"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
		go build -trimpath -ldflags "$LDFLAGS" -o "$stage/$bin" ./cmd/levier
	chmod 0755 "$stage/$bin"

	if [ "$goos" = "windows" ]; then
		archive="$OUT_DIR/$name.zip"
		zip_dir "$stage" "$archive"
	else
		archive="$OUT_DIR/$name.tar.gz"
		tar_dir "$stage" "$archive" "$bin"
	fi
}

build_target linux amd64
build_target linux arm64
build_target darwin amd64
build_target darwin arm64
build_target windows amd64

(
	cd "$OUT_DIR"
	: >checksums.txt
	for archive in *.tar.gz *.zip; do
		[ -f "$archive" ] || continue
		printf '%s  %s\n' "$(sha256_file "$archive")" "$archive" >>checksums.txt
	done
)

echo
echo "release artifacts: $OUT_DIR"
cat "$OUT_DIR/checksums.txt"
