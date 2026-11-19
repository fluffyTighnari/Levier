#!/bin/sh
# Levier — restore PostgreSQL and the agent credentials volume from a backup.
#
# Usage:
#   scripts/restore.sh --backup DIR --yes [options]
#
# Options:
#   --backup DIR        backup directory created by scripts/backup.sh
#   --yes               confirm destructive restore
#   --dry-run           verify and print the plan without changing services
#   --skip-agent-data   restore only PostgreSQL
#   --smoke             run the Compose smoke profile after restore
#   --project-name NAME set COMPOSE_PROJECT_NAME for docker compose
#   -h, --help          show this help
#
# Restore stops levier, harness, and agent before changing the database. The
# script does not restore deploy/secrets. Keep those separately.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
umask 077

# Git Bash/MSYS otherwise rewrites container paths such as /app and /data to
# Windows paths before they reach docker. Ignored by non-MSYS shells.
MSYS_NO_PATHCONV=1
export MSYS_NO_PATHCONV

PGUSER=${PGUSER:-levier}
PGDATABASE=${PGDATABASE:-levier}
BACKUP=""
ASSUME_YES=0
DRY_RUN=0
SKIP_AGENT=0
RUN_SMOKE=0

usage() {
	cat <<'EOF'
usage: scripts/restore.sh --backup DIR --yes [options]

options:
  --backup DIR        directory created by scripts/backup.sh
  --yes               confirm destructive restore
  --dry-run           verify and print the plan only
  --skip-agent-data   restore only PostgreSQL
  --smoke             run the Compose smoke profile after restore
  --project-name NAME set COMPOSE_PROJECT_NAME for docker compose
  -h, --help          show this help

The script stops levier/harness/agent, restores PostgreSQL and optionally the
agent_data volume, then restarts them. It never restores deploy/secrets.
EOF
}

die() {
	printf 'restore: %s\n' "$*" >&2
	exit 1
}

compose() {
	docker compose "$@"
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--backup)
		[ "$#" -ge 2 ] || die "missing value for --backup"
		BACKUP=$2
		shift 2
		;;
	--yes)
		ASSUME_YES=1
		shift
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--skip-agent-data)
		SKIP_AGENT=1
		shift
		;;
	--smoke)
		RUN_SMOKE=1
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

[ -n "$BACKUP" ] || die "--backup DIR is required"
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

case "$BACKUP" in
	/*) ;;
	*) BACKUP="$ROOT/$BACKUP" ;;
esac
[ -d "$BACKUP" ] || die "backup directory not found: $BACKUP"
[ -f "$BACKUP/manifest.txt" ] || die "missing manifest.txt"
[ -f "$BACKUP/postgres.dump" ] || die "missing postgres.dump"
[ -f "$BACKUP/SHA256SUMS" ] || die "missing SHA256SUMS"
grep -qx 'format=levier-backup-v1' "$BACKUP/manifest.txt" ||
	die "unsupported backup format"

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$BACKUP" && sha256sum -c SHA256SUMS >/dev/null) ||
		die "backup checksum verification failed"
elif command -v shasum >/dev/null 2>&1; then
	(cd "$BACKUP" && shasum -a 256 -c SHA256SUMS >/dev/null) ||
		die "backup checksum verification failed"
else
	die "sha256sum or shasum is required"
fi

agent_backup=""
if [ "$SKIP_AGENT" -eq 0 ] && grep -qx 'agent_data=agent_data.tar' "$BACKUP/manifest.txt"; then
	[ -f "$BACKUP/agent_data.tar" ] || die "manifest lists agent_data.tar but it is missing"
	agent_backup="$BACKUP/agent_data.tar"
fi

printf 'restore backup: %s\n' "$BACKUP"
printf 'database: %s\n' "$PGDATABASE"
printf 'agent_data: %s\n' "${agent_backup:-skip}"
printf 'secrets: not included\n'

if [ "$DRY_RUN" -eq 1 ]; then
	printf 'restore: dry-run; no services or data changed\n'
	exit 0
fi
[ "$ASSUME_YES" -eq 1 ] || die "restore is destructive; pass --yes after reviewing --dry-run"

command -v docker >/dev/null 2>&1 || die "docker is required"
docker compose version >/dev/null 2>&1 || die "docker compose v2 is required"

running=$(compose ps --status running --services 2>/dev/null || true)
if ! printf '%s\n' "$running" | grep -qx postgres; then
	printf 'restore: starting postgres\n' >&2
	compose up -d postgres
fi

printf 'restore: stopping writers\n' >&2
compose stop levier harness agent >/dev/null || true

printf 'restore: terminating remaining database sessions\n' >&2
compose exec -T postgres psql -U "$PGUSER" -d postgres -v ON_ERROR_STOP=1 -c \
	"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$PGDATABASE' AND pid <> pg_backend_pid();" \
	>/dev/null || true

printf 'restore: restoring PostgreSQL\n' >&2
if ! compose exec -T postgres pg_restore \
	-U "$PGUSER" -d "$PGDATABASE" --clean --if-exists --no-owner --no-privileges \
	< "$BACKUP/postgres.dump"; then
	printf 'restore: PostgreSQL restore failed; services remain stopped\n' >&2
	printf 'restore: keep %s and inspect the pg_restore output above\n' "$BACKUP" >&2
	exit 1
fi

if [ -n "$agent_backup" ]; then
	printf 'restore: restoring agent_data\n' >&2
	if ! compose run --rm --no-deps -T --entrypoint sh agent -c \
		'find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C /data -xf -' \
		< "$agent_backup"; then
		printf 'restore: agent_data restore failed; services remain stopped\n' >&2
		exit 1
	fi
fi

printf 'restore: restarting services\n' >&2
compose up -d levier harness agent

status=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
	if status=$(compose exec -T levier /app/levier migrate status 2>&1); then
		break
	fi
	status=""
	sleep 2
done
if [ -z "$status" ]; then
	printf 'restore: warning: could not read migration status yet\n' >&2
else
	printf '%s\n' "$status"
	case "$status" in
		*"pending: 0"*) ;;
		*) printf 'restore: warning: migrations are not fully applied\n' >&2 ;;
	esac
fi

if [ "$RUN_SMOKE" -eq 1 ]; then
	printf 'restore: running smoke\n' >&2
	compose --profile smoke run --rm smoke
fi

printf 'restore: complete\n'
