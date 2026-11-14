#!/bin/sh
# Middleware — container entrypoint; migrate then serve.
set -eu

CONFIG="${MIDDLEWARE_CONFIG:-/app/config.example.yaml}"

until /app/migrate -config "$CONFIG"; do
  echo "middleware: waiting for PostgreSQL" >&2
  sleep 1
done

exec /app/middleware -config "$CONFIG"
