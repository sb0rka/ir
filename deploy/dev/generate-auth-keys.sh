#!/usr/bin/env bash
# Run once before starting ir-api. ir-api runs as non-root and bind-mounts these
# files read-only, so they must be world-readable (same as Sb0rka keygen).
#
# If compose was started before the files existed, Docker creates empty
# directories as bind stubs owned by root. This script removes those stubs via
# Docker and recreates real key files owned by the current user.
set -euo pipefail

ir_key_dir="${IR_AUTH_KEY_DIR:-.local/auth}"
ir_key_dir="$(cd "$(dirname "$ir_key_dir")" && pwd)/$(basename "$ir_key_dir")"
ir_parent="$(dirname "$ir_key_dir")"
owner_uid="$(id -u)"
owner_gid="$(id -g)"

needs_reset=0
if [ -e "$ir_key_dir" ] && [ ! -d "$ir_key_dir" ]; then
  echo "[generate-auth-keys] $ir_key_dir must be a directory" >&2
  exit 1
fi
if [ -d "$ir_key_dir" ]; then
  if [ ! -O "$ir_key_dir" ] || [ ! -w "$ir_key_dir" ]; then
    needs_reset=1
  fi
  for key in ir-session ir-oidc-context; do
    if [ -d "$ir_key_dir/$key.key" ]; then
      needs_reset=1
    fi
  done
fi

if [ "$needs_reset" -eq 1 ]; then
  if ! command -v docker >/dev/null 2>&1; then
    echo "[generate-auth-keys] $ir_key_dir has Docker bind stubs or is not writable." >&2
    echo "Remove it (including root-owned *.key directories) and rerun." >&2
    exit 1
  fi
  echo "[generate-auth-keys] resetting unusable $ir_key_dir"
  mkdir -p "$ir_parent"
  docker run --rm \
    -v "$ir_parent:/local" \
    -e "TARGET=$(basename "$ir_key_dir")" \
    -e "OWNER_UID=$owner_uid" \
    -e "OWNER_GID=$owner_gid" \
    alpine:3.21 \
    sh -c 'rm -rf "/local/$TARGET" && mkdir -p "/local/$TARGET" && chown "$OWNER_UID:$OWNER_GID" "/local/$TARGET" && chmod 0700 "/local/$TARGET"'
fi

mkdir -p "$ir_key_dir"
if [ -O "$ir_key_dir" ]; then
  chmod 0700 "$ir_key_dir"
fi

for key in ir-session ir-oidc-context; do
  ir_key_path="$ir_key_dir/$key.key"
  if [ -d "$ir_key_path" ]; then
    echo "[generate-auth-keys] $ir_key_path is still a directory after reset" >&2
    exit 1
  fi
  if [ ! -e "$ir_key_path" ]; then
    openssl rand -base64 32 > "$ir_key_path"
  fi
  test -f "$ir_key_path" && test -s "$ir_key_path"
  chmod 0644 "$ir_key_path"
done

echo "[generate-auth-keys] ready in $ir_key_dir"
