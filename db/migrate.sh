#!/usr/bin/env sh

set -eu

if [ "$#" -eq 0 ]; then
  echo "at least one script path is required"
  exit 1
fi

POSTGRES_HOST="${POSTGRES_HOST:?POSTGRES_HOST is required}"
POSTGRES_PORT="${POSTGRES_PORT:?POSTGRES_PORT is required}"
POSTGRES_USER="${POSTGRES_USER:?POSTGRES_USER is required}"
POSTGRES_DB="${POSTGRES_DB:?POSTGRES_DB is required}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"
DB_INV_SCHEMA_NAME="${DB_INV_SCHEMA_NAME:-}"
DB_ADMIN_PASSWORD="${DB_ADMIN_PASSWORD:?DB_ADMIN_PASSWORD is required}"

PGPASSFILE_PATH="/tmp/.pgpass"
export PGPASSFILE="${PGPASSFILE_PATH}"

printf '%s:%s:%s:%s:%s\n' \
  "${POSTGRES_HOST}" \
  "${POSTGRES_PORT}" \
  "${POSTGRES_DB}" \
  "${POSTGRES_USER}" \
  "${POSTGRES_PASSWORD}" > "${PGPASSFILE_PATH}"

chmod 600 "${PGPASSFILE_PATH}"
trap 'rm -f "${PGPASSFILE_PATH}"' EXIT

run_script() {
  SCRIPT_PATH="$1"

  set -- \
    --no-password \
    --host "${POSTGRES_HOST}" \
    --port "${POSTGRES_PORT}" \
    --username "${POSTGRES_USER}" \
    --dbname "${POSTGRES_DB}" \
    --set=ON_ERROR_STOP=1 \
    --set=POSTGRES_HOST="${POSTGRES_HOST}" \
    --set=POSTGRES_PORT="${POSTGRES_PORT}" \
    --set=POSTGRES_DB="${POSTGRES_DB}"

  if [ -n "${DB_INV_SCHEMA_NAME}" ]; then
    set -- "$@" --set=DB_INV_SCHEMA_NAME="${DB_INV_SCHEMA_NAME}"
  fi

  set -- "$@" \
    --set=DB_ADMIN_PASSWORD="${DB_ADMIN_PASSWORD}" \
    -f "${SCRIPT_PATH}"

  echo "Applying ${SCRIPT_PATH} to ${POSTGRES_DB}"
  PGOPTIONS="${PGOPTIONS:-}" psql "$@"
}

for script_path in "$@"; do
  if [ ! -f "${script_path}" ]; then
    echo "migration file not found: ${script_path}"
    exit 1
  fi

  run_script "${script_path}"
done
