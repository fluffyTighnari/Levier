#!/bin/sh
# Levier — install a release binary.
#
# Usage:
#   sh scripts/install.sh
#   sh scripts/install.sh --version v0.1.0 --prefix ~/.local/bin
#   sh scripts/install.sh --version v0.1.0 --base-url http://127.0.0.1:8000
set -eu

DEFAULT_REPO="fluffyTighnari/levier"
REPO=${LEVIER_REPO:-$DEFAULT_REPO}
VERSION=""
PREFIX=""
BASE_URL=""
DRY_RUN=0

usage() {
	cat <<'EOF'
usage: install.sh [options]

options:
  --version <tag>     install a specific release, for example v0.1.0
  --prefix <dir>      install directory; defaults to /usr/local/bin or ~/.local/bin
  --repo <owner/repo> GitHub repository; default fluffyTighnari/levier
  --base-url <url>    mirror/release directory URL; requires --version
  --dry-run           print the plan without downloading or writing
  -h, --help          show this help

environment:
  LEVIER_REPO         same as --repo

The installer always downloads checksums.txt and verifies the archive before
installing. It does not edit shell startup files.
EOF
}

die() {
	printf 'install: %s\n' "$*" >&2
	exit 2
}

fail() {
	printf 'install: %s\n' "$*" >&2
	exit 1
}

need_value() {
	if [ "$#" -lt 2 ] || [ -z "$2" ]; then
		die "missing value for $1"
	fi
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		need_value "$@"
		VERSION=$2
		shift 2
		;;
	--prefix)
		need_value "$@"
		PREFIX=$2
		shift 2
		;;
	--repo)
		need_value "$@"
		REPO=$2
		shift 2
		;;
	--base-url)
		need_value "$@"
		BASE_URL=$2
		shift 2
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'install: unknown argument: %s\n\n' "$1" >&2
		usage >&2
		exit 2
		;;
	esac
done

case "$REPO" in
	*/*) ;;
	*) die "repository must look like owner/name" ;;
esac

if [ -n "$BASE_URL" ] && [ -z "$VERSION" ]; then
	die "--base-url requires --version"
fi

if [ -n "$VERSION" ]; then
	case "$VERSION" in
		v*) ;;
		*) VERSION="v$VERSION" ;;
	esac
fi

os=$(uname -s 2>/dev/null || true)
case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*)
		die "unsupported OS: ${os:-unknown}; Windows users should run install.ps1"
		;;
esac

arch=$(uname -m 2>/dev/null || true)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "unsupported architecture: ${arch:-unknown}" ;;
esac

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" || fail "download failed: $1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO- "$1" || fail "download failed: $1"
	else
		fail "curl or wget is required"
	fi
}

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1" || fail "download failed: $1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1" || fail "download failed: $1"
	else
		fail "curl or wget is required"
	fi
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		value=$(sha256sum "$1") || fail "cannot hash $1"
	elif command -v shasum >/dev/null 2>&1; then
		value=$(shasum -a 256 "$1") || fail "cannot hash $1"
	elif command -v openssl >/dev/null 2>&1; then
		value=$(openssl dgst -sha256 -r "$1") || fail "cannot hash $1"
	else
		fail "sha256sum, shasum or openssl is required"
	fi
	printf '%s\n' "$value" | awk '{print $1}'
}

if [ -z "$VERSION" ]; then
	api_url="https://api.github.com/repos/$REPO/releases/latest"
	VERSION=$(fetch "$api_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	if [ -z "$VERSION" ]; then
		fail "could not resolve latest release; pass --version"
	fi
fi

if [ -z "$BASE_URL" ]; then
	release_base="https://github.com/$REPO/releases/download/$VERSION"
else
	BASE_URL=${BASE_URL%/}
	release_base=$BASE_URL
fi

archive="levier_${VERSION}_${os}_${arch}.tar.gz"
if [ -z "$PREFIX" ]; then
	if [ "$(id -u)" -eq 0 ] || [ -w /usr/local/bin ]; then
		PREFIX=/usr/local/bin
	else
		PREFIX="$HOME/.local/bin"
	fi
fi

case "$PREFIX" in
	"~/"*) PREFIX="$HOME/${PREFIX#\~/}" ;;
	/*) ;;
	*) PREFIX="$(pwd)/$PREFIX" ;;
esac

printf 'repository: %s\n' "$REPO"
printf 'version:    %s\n' "$VERSION"
printf 'platform:   %s/%s\n' "$os" "$arch"
printf 'archive:    %s\n' "$archive"
printf 'url:        %s/%s\n' "$release_base" "$archive"
printf 'prefix:     %s\n' "$PREFIX"

if [ "$DRY_RUN" -eq 1 ]; then
	printf 'dry-run:    no download or write\n'
	exit 0
fi

TMP=$(mktemp -d "${TMPDIR:-/tmp}/levier-install.XXXXXX") || fail "cannot create temporary directory"
trap 'rm -rf "$TMP"' EXIT INT TERM

download "$release_base/$archive" "$TMP/$archive"
download "$release_base/checksums.txt" "$TMP/checksums.txt"

expected=$(awk -v asset="$archive" '$2 == asset {print $1; exit}' "$TMP/checksums.txt" | tr -d '\r')
if [ -z "$expected" ]; then
	fail "checksums.txt does not contain $archive"
fi
actual=$(sha256_file "$TMP/$archive")
if [ "$expected" != "$actual" ]; then
	fail "checksum mismatch for $archive: expected $expected, got $actual"
fi

mkdir -p "$TMP/extract" || fail "cannot create extraction directory"
tar -xzf "$TMP/$archive" -C "$TMP/extract" || fail "cannot extract $archive"
source_bin="$TMP/extract/levier"
if [ ! -f "$source_bin" ]; then
	fail "archive does not contain levier"
fi

mkdir -p "$PREFIX" || fail "cannot create install directory: $PREFIX"
target="$PREFIX/levier"
staged="$PREFIX/.levier.tmp.$$"
cp "$source_bin" "$staged" || fail "cannot stage levier"
chmod 0755 "$staged" || fail "cannot mark levier executable"
mv "$staged" "$target" || fail "cannot install levier to $target"

printf 'installed:  %s\n' "$target"
case ":$PATH:" in
	*":$PREFIX:"*) ;;
	*)
		printf '\nPATH hint:\n  export PATH="%s:$PATH"\n' "$PREFIX"
		;;
esac
