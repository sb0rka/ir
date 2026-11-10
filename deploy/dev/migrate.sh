#!/usr/bin/env bash
set -euo pipefail

export PGPASSWORD=postgres
PSQL=(psql -v ON_ERROR_STOP=1 -h postgres -U postgres -d investigations)

echo "[migrate] applying investigations migrations..."
for f in /migrations/*.sql; do
  echo "[migrate]   -> $(basename "$f")"
  "${PSQL[@]}" -v DB_INV_SCHEMA_NAME=inv -f "$f"
done

echo "[migrate] applying seed..."
for f in /migrations-seed/*.sql; do
  echo "[migrate]   -> $(basename "$f")"
  "${PSQL[@]}" -v DB_INV_SCHEMA_NAME=inv -f "$f"
done

echo "[migrate] done"
