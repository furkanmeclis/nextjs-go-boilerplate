#!/bin/sh
set -eu

ROLE="${APP_ROLE:-server}"

if [ "${ROLE}" = "server" ] && [ "${AUTO_MIGRATE:-true}" = "true" ]; then
  : "${DB_HOST:?DB_HOST is required for migrate}"
  : "${DB_PORT:?DB_PORT is required for migrate}"
  : "${DB_USER:?DB_USER is required for migrate}"
  : "${DB_PASSWORD:?DB_PASSWORD is required for migrate}"
  : "${DB_NAME:?DB_NAME is required for migrate}"
  DB_SSLMODE="${DB_SSLMODE:-disable}"

  DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"

  echo "migrate: applying migrations..."
  migrate -path /app/migrations -database "${DB_URL}" up
  echo "migrate: done"
fi

case "${ROLE}" in
  worker)
    exec /app/worker
    ;;
  migrate)
    : "${DATABASE_URL:?DATABASE_URL is required}"
    migrate -path /app/migrations -database "${DATABASE_URL}" up
    # One app_releases row per deploy (version/sha/notes baked into this
    # image). Fail-soft: it exits 0 on any problem; `|| true` is belt+braces.
    /app/release-record || true
    ;;
  *)
    exec /app/server
    ;;
esac
