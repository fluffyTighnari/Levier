#!/bin/sh
# Levier — backup PostgreSQL and the agent credentials volume.
#
# Usage:
#   scripts/backup.sh [options]
#
# Options:
#   --output DIR        output directory (default data/backups/levier-<UTC time>)
#   --db-only           skip the agent_data volume
#   --project-name NAME set COMPOSE_PROJECT_NAME for docker compose
#   -h, --help          show this help
#
# Environment:
#   COMPOSE_FILE        optional Compose file list, as accepted by docker compose
#   COMPOSE_PROJECT_NAME
#   PGUSER              database user (default levier)
#   PGDATABASE          database name (default levier)
#
# The backup deliberately does not include deploy/secrets. Keep those files
# separate, encrypted, and access-controlled.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
umask 077

# Git Bash/MSYS otherwise rewrites container paths such as /data to Windows
# paths before they reach docker. Ignored by non-MSYS shells.
MSYS_NO_PATHCONV=1
export MSYS_NO_PATHCONV

PGUSER=${PGUSER:-levier}
PGDATABASE=${PGDATABASE:-levier}
OUTPUT=""
DB_ONLY=0

usage() {
	cat <<'EOF'
usage: scripts/backup.sh [options]

options:
  --output DIR        output directory (default data/backups/levier-<UTC time>)
  --db-only           skip the agent_data volume
  --project-name NAME set COMPOSE_PROJECT_NAME for docker compose
  -h, --help          show this help

The backup contains postgres.dump, an optional agent_data.tar, manifest.txt,
and SHA256SUMS. deploy/secrets is never included.
EOF
}

die() {
	printf 'backup: %s\n' "$*" >&2
	exit 1
}

compose() {
	docker compose "$@"
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "sha256sum or shasum is required"
	fi
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--output)
		[ "$#" -ge 2 ] || die "missing value for --output"
		OUTPUT=$2
		shift 2
		;;
	--db-only)
		DB_ONLY=1
		shift
		;;
	--project-name)
		[ "$#" -ge 2 ] || die "missing value for --project-name"
		COMPOSE_PROJECT_NAME=$2
		export COMPOSE_PROJECT_NAME
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "unknown option: $1"
		;;
	esac
done

case "$PGUSER" in
	'' | *[!A-Za-z0-9_]*)
		die "PGUSER must contain only letters, digits, and underscore"
		;;
esac
case "$PGDATABASE" in
	'' | *[!A-Za-z0-9_]*)
		die "PGDATABASE must contain only letters, digits, and underscore"
		;;
esac

command -v docker >/dev/null 2>&1 || die "docker is required"
docker compose version >/dev/null 2>&1 || die "docker compose v2 is required"

if [ -z "$OUTPUT" ]; then
	OUTPUT="data/backups/levier-$(date -u +%Y%m%dT%H%M%SZ)"
fi
case "$OUTPUT" in
	/*) ;;
	*) OUTPUT="$ROOT/$OUTPUT" ;;
esac
[ "$OUTPUT" != "/" ] || die "invalid output directory"
[ -e "$OUTPUT" ] && die "output already exists: $OUTPUT"

mkdir -p "$(dirname "$OUTPUT")"
TMP="${OUTPUT}.partial.$$"
cleanup() {
	rm -rf "$TMP"
}
trap cleanup EXIT INT TERM
mkdir -p "$TMP"
chmod 700 "$TMP"

running=$(compose ps --status running --services 2>/dev/null || true)
printf '%s\n' "$running" | grep -qx postgres ||
	die "postgres service is not running; start the stack first"

migration_version=$(compose exec -T postgres \
	psql -U "$PGUSER" -d "$PGDATABASE" -Atc \
	"SELECT COALESCE(max(version), 0) FROM schema_migrations" 2>/dev/null || true)
[ -n "$migration_version" ] || migration_version=unknown

printf 'backup: dumping PostgreSQL database %s\n' "$PGDATABASE" >&2
if ! compose exec -T postgres pg_dump \
	-U "$PGUSER" -d "$PGDATABASE" -Fc --no-owner --no-privileges \
	> "$TMP/postgres.dump"; then
	die "pg_dump failed"
fi
[ -s "$TMP/postgres.dump" ] || die "pg_dump produced an empty file"

agent_data=not-included
if [ "$DB_ONLY" -eq 0 ]; then
	printf '%s\n' "$running" | grep -qx agent ||
		die "agent service is not running; use --db-only to skip agent credentials"
	printf 'backup: copying agent_data volume\n' >&2
	if ! compose exec -T agent tar -C /data -cf - . > "$TMP/agent_data.tar"; then
		die "agent_data copy failed"
	fi
	[ -s "$TMP/agent_data.tar" ] || die "agent_data copy produced an empty file"
	agent_data=agent_data.tar
fi

cat > "$TMP/manifest.txt" <<EOF
format=levier-backup-v1
created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
migration_version=$migration_version
database=$PGDATABASE
pg_user=$PGUSER
compose_project=${COMPOSE_PROJECT_NAME:-compose-default}
postgres_dump=postgres.dump
agent_data=$agent_data
secrets=not-included
EOF

(
	cd "$TMP"
	: > SHA256SUMS
	for file in postgres.dump agent_data.tar manifest.txt; do
		[ -f "$file" ] || continue
		printf '%s  %s\n' "$(sha256_file "$file")" "$file" >> SHA256SUMS
	done
)

chmod 600 "$TMP"/* 2>/dev/null || true
mv "$TMP" "$OUTPUT"
trap - EXIT INT TERM
chmod 700 "$OUTPUT"

printf 'backup complete: %s\n' "$OUTPUT"
printf 'migration version: %s\n' "$migration_version"
printf 'agent_data: %s\n' "$agent_data"
printf 'secrets: not included; back up deploy/secrets separately\n'
