#!/bin/sh
set -eu

config="${OPENVPN_CONFIG:-/vpn/vpn.conf}"
if [ ! -f "$config" ]; then
  echo "openvpn config not found: $config" >&2
  exit 1
fi

exec openvpn \
  --config "$config" \
  --dev tun \
  --script-security 2 \
  --verb 3
