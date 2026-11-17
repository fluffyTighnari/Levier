#!/bin/sh
# Levier — container entrypoint; migrate then serve.
set -eu

CONFIG="${LEVIER_CONFIG:-${MIDDLEWARE_CONFIG:-/app/levier.example.yaml}}"

/app/levier migrate up -config "$CONFIG"

exec /app/levier serve -config "$CONFIG"
