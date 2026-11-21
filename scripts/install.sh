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
MODIFY_PATH=1
WITH_STACK=1
WITH_SDK=1

usage() {
	cat <<'EOF'
usage: install.sh [options]

options:
  --version <tag>     install a specific release, for example v0.1.0
  --prefix <dir>      install directory; defaults to /usr/local/bin or ~/.local/bin
  --repo <owner/repo> GitHub repository; default fluffyTighnari/levier
  --base-url <url>    mirror/release directory URL; requires --version
  --no-modify-path    do not add the install directory to a shell profile
  --no-stack          do not download the Docker Compose source bundle
  --no-sdk            do not download/install the Python App SDK wheel bundle
  --dry-run           print the plan without downloading or writing
  -h, --help          show this help

environment:
  LEVIER_REPO         same as --repo
  LEVIER_SOURCE_URL   override the Compose source archive URL

The installer always downloads checksums.txt and verifies the archive before
installing. By default it adds the install directory to the current user's
shell profile, installs the Compose source bundle, and installs the Python SDK
from the release wheel bundle, so `levier`, `levier up`, and
`from levier import App, tool, Runtime` work from any directory.
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
	--no-modify-path)
		MODIFY_PATH=0
		shift
		;;
	--no-stack)
		WITH_STACK=0
		shift
		;;
	--no-sdk)
		WITH_SDK=0
		shift
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

levier_data_dir() {
	if [ -n "${LEVIER_HOME:-}" ]; then
		printf '%s\n' "$LEVIER_HOME"
		return 0
	fi
	case "$(uname -s 2>/dev/null || true)" in
		Darwin) printf '%s\n' "$HOME/Library/Application Support/levier" ;;
		*)
			if [ -n "${XDG_DATA_HOME:-}" ]; then
				printf '%s\n' "$XDG_DATA_HOME/levier"
			else
				printf '%s\n' "$HOME/.local/share/levier"
			fi
			;;
	esac
}

install_stack() {
	data_dir=$(levier_data_dir)
	stack_dir="$data_dir/stack"
	source_url=${LEVIER_SOURCE_URL:-"https://github.com/$REPO/archive/refs/tags/$VERSION.tar.gz"}
	work=$(mktemp -d "${TMPDIR:-/tmp}/levier-stack.XXXXXX") || return 1
	archive="$work/source.tar.gz"
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$archive" "$source_url" 2>/dev/null || {
			rm -rf "$work"
			return 1
		}
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$archive" "$source_url" 2>/dev/null || {
			rm -rf "$work"
			return 1
		}
	else
		rm -rf "$work"
		return 1
	fi
	if ! tar -xzf "$archive" -C "$work" 2>/dev/null; then
		rm -rf "$work"
		return 1
	fi
	compose=$(find "$work" -name docker-compose.yml -type f | head -n 1)
	if [ -z "$compose" ]; then
		rm -rf "$work"
		return 1
	fi
	source_dir=$(dirname "$compose")
	mkdir -p "$stack_dir" || {
		rm -rf "$work"
		return 1
	}
	cp -R "$source_dir/." "$stack_dir/" || {
		rm -rf "$work"
		return 1
	}
	rm -rf "$work"
	printf '%s\n' "$stack_dir/docker-compose.yml"
}

install_sdk() {
	data_dir=$(levier_data_dir)
	wheels_dir="$data_dir/python-wheels"
	work=$(mktemp -d "${TMPDIR:-/tmp}/levier-sdk.XXXXXX") || return 1
	archive="$work/wheels.tar.gz"
	url="$release_base/levier-python-wheels.tar.gz"
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$archive" "$url" 2>/dev/null || {
			rm -rf "$work"
			return 1
		}
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$archive" "$url" 2>/dev/null || {
			rm -rf "$work"
			return 1
		}
	else
		rm -rf "$work"
		return 1
	fi
	mkdir -p "$wheels_dir" || {
		rm -rf "$work"
		return 1
	}
	if ! tar -xzf "$archive" -C "$wheels_dir" 2>/dev/null; then
		rm -rf "$work"
		return 1
	fi
	rm -rf "$work"

	python_bin=""
	if command -v python3 >/dev/null 2>&1; then
		python_bin=python3
	elif command -v python >/dev/null 2>&1; then
		python_bin=python
	fi
	if [ -n "$python_bin" ]; then
		sdk_installed=0
		if "$python_bin" -m pip install --user --no-index --disable-pip-version-check \
			--find-links "$wheels_dir" levier >/dev/null 2>&1; then
			sdk_installed=1
		elif "$python_bin" -m pip install --no-index --disable-pip-version-check \
			--find-links "$wheels_dir" levier >/dev/null 2>&1; then
			sdk_installed=1
		fi
		if [ "$sdk_installed" -eq 1 ]; then
			printf 'sdk:        %s\n' "$wheels_dir" >&2
			printf 'sdk Python: %s (from levier import App, tool, Runtime)\n' "$python_bin" >&2
		else
			printf 'warning: could not install levier; install it from %s\n' "$wheels_dir" >&2
		fi
	else
		printf 'warning: Python was not found; install levier later from %s\n' "$wheels_dir" >&2
	fi
	printf '%s\n' "$wheels_dir"
}

add_to_path() {
	case ":$PATH:" in
		*":$PREFIX:"*) return 0 ;;
	esac
	if [ "$MODIFY_PATH" -eq 0 ]; then
		printf '\nPATH hint:\n  export PATH="%s:$PATH"\n' "$PREFIX"
		return 0
	fi

	shell_name=$(basename "${SHELL:-sh}")
	case "$shell_name" in
		zsh)
			profile="${ZDOTDIR:-$HOME}/.zshrc"
			;;
		fish)
			profile="${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish"
			;;
		bash)
			if [ "$(uname -s 2>/dev/null || true)" = "Darwin" ]; then
				profile="$HOME/.bash_profile"
			else
				profile="$HOME/.bashrc"
			fi
			;;
		*)
			profile="$HOME/.profile"
			;;
	esac

	if ! mkdir -p "$(dirname "$profile")" 2>/dev/null; then
		printf '\nPATH hint:\n  export PATH="%s:$PATH"\n' "$PREFIX"
		return 0
	fi
	if grep -F "Added by Levier installer" "$profile" >/dev/null 2>&1 &&
		grep -F "$PREFIX" "$profile" >/dev/null 2>&1; then
		printf '\nPATH already configured in %s; open a new terminal.\n' "$profile"
		return 0
	fi
	if ! touch "$profile" 2>/dev/null; then
		printf '\nPATH hint:\n  export PATH="%s:$PATH"\n' "$PREFIX"
		return 0
	fi

	if [ "$shell_name" = "fish" ]; then
		printf '\n# Added by Levier installer: make levier available from any directory.\nfish_add_path --prepend "%s"\n' "$PREFIX" >>"$profile"
	else
		{
			printf '\n# Added by Levier installer: make levier available from any directory.\n'
			printf 'case ":$PATH:" in\n'
			printf '  *":%s:"*) ;;\n' "$PREFIX"
			printf '  *) export PATH="%s:$PATH" ;;\n' "$PREFIX"
			printf 'esac\n'
		} >>"$profile"
	fi
	printf '\nPATH updated in %s; open a new terminal or run:\n  export PATH="%s:$PATH"\n' "$profile" "$PREFIX"
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
stack_file=""
if [ "$WITH_STACK" -eq 1 ]; then
	stack_file=$(install_stack) || {
		printf 'warning: Compose source bundle was not installed; use --file with levier stack\n' >&2
	}
	if [ -n "$stack_file" ]; then
		printf 'stack:      %s\n' "$stack_file"
	fi
fi
sdk_dir=""
if [ "$WITH_SDK" -eq 1 ]; then
	sdk_dir=$(install_sdk) || {
		printf 'warning: Python SDK wheel bundle was not installed; use --no-sdk to skip it\n' >&2
	}
	if [ -n "$sdk_dir" ]; then
		printf 'wheels:     %s\n' "$sdk_dir"
	fi
fi
add_to_path
printf '\nNext:\n  levier setup'
if [ -n "$stack_file" ]; then
	printf ' --stack-file "%s"' "$stack_file"
fi
printf '\n  levier status\n'
if [ -n "$stack_file" ]; then
	printf '  levier stack up --build\n'
fi
