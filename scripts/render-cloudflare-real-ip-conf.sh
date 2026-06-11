#!/usr/bin/env bash

set -euo pipefail

OUTPUT_PATH="${1:-}"

if [[ -z "$OUTPUT_PATH" ]]; then
  echo "Usage: render-cloudflare-real-ip-conf.sh <output-path>" >&2
  exit 1
fi

command -v curl >/dev/null 2>&1 || {
  echo "curl is required to fetch Cloudflare IP ranges." >&2
  exit 1
}

TMP_FILE="$(mktemp)"

cleanup() {
  rm -f "$TMP_FILE"
}
trap cleanup EXIT

{
  echo "# Generated from Cloudflare IP ranges. Refresh when Cloudflare publishes range changes."
  echo "# Sources: https://www.cloudflare.com/ips-v4 and https://www.cloudflare.com/ips-v6"
  echo "# This restores visitor IP only; enforce Cloudflare-only origin access in the VPS firewall or security group."
  for url in https://www.cloudflare.com/ips-v4 https://www.cloudflare.com/ips-v6; do
    curl -fsSL --retry 3 "$url" | awk 'NF && $1 !~ /^#/ { print "set_real_ip_from " $1 ";" }'
  done
  echo "real_ip_header CF-Connecting-IP;"
  echo "real_ip_recursive on;"
} > "$TMP_FILE"

if ! grep -q '^set_real_ip_from ' "$TMP_FILE"; then
  echo "Cloudflare IP range response was empty." >&2
  exit 1
fi

mkdir -p "$(dirname "$OUTPUT_PATH")"
mv "$TMP_FILE" "$OUTPUT_PATH"
trap - EXIT

echo "Rendered Cloudflare real IP config: $OUTPUT_PATH"
