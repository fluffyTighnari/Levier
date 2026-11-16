#!/bin/sh
# Middleware — container entrypoint; migrate then serve.
set -eu

CONFIG="${LEVIER_CONFIG:-${MIDDLEWARE_CONFIG:-/app/levier.example.yaml}}"

until /app/migrate -config "$CONFIG"; do
  echo "levier: waiting for PostgreSQL" >&2
  sleep 1
done

exec /app/levier -config "$CONFIG"
