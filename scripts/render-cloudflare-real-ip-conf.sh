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

OUTPUT_DIR="$(dirname "$OUTPUT_PATH")"
mkdir -p "$OUTPUT_DIR"
TMP_FILE="$(mktemp "$OUTPUT_DIR/.cloudflare-real-ip.XXXXXX")"
TMP_RANGES="$(mktemp)"

cleanup() {
  rm -f "$TMP_FILE" "$TMP_RANGES"
}
trap cleanup EXIT

is_valid_cidr() {
  local cidr="$1"
  local prefix octet ip

  if [[ "$cidr" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/([0-9]|[12][0-9]|3[0-2])$ ]]; then
    ip="${cidr%/*}"
    IFS=. read -r -a octets <<< "$ip"
    for octet in "${octets[@]}"; do
      if ((10#$octet > 255)); then
        return 1
      fi
    done
    return 0
  fi

  if [[ "$cidr" =~ ^[0-9A-Fa-f:]+/[0-9]{1,3}$ ]]; then
    prefix="${cidr##*/}"
    ((10#$prefix <= 128))
    return
  fi

  return 1
}

for url in https://www.cloudflare.com/ips-v4 https://www.cloudflare.com/ips-v6; do
  curl -fsSL --retry 3 --connect-timeout 5 --max-time 20 "$url" >> "$TMP_RANGES"
  printf '\n' >> "$TMP_RANGES"
done

CIDRS=()
while IFS= read -r cidr; do
  CIDRS+=("$cidr")
done < <(awk 'NF && $1 !~ /^#/ { print $1 }' "$TMP_RANGES")

if ((${#CIDRS[@]} == 0)); then
  echo "Cloudflare IP range response was empty." >&2
  exit 1
fi

for cidr in "${CIDRS[@]}"; do
  if ! is_valid_cidr "$cidr"; then
    echo "Cloudflare IP range response contained invalid CIDR: $cidr" >&2
    exit 1
  fi
done

{
  echo "# Generated from Cloudflare IP ranges. Refresh when Cloudflare publishes range changes."
  echo "# Sources: https://www.cloudflare.com/ips-v4 and https://www.cloudflare.com/ips-v6"
  echo "# This restores visitor IP only; enforce Cloudflare-only origin access in the VPS firewall or security group."
  for cidr in "${CIDRS[@]}"; do
    echo "set_real_ip_from $cidr;"
  done
  echo "real_ip_header CF-Connecting-IP;"
  echo "real_ip_recursive on;"
} > "$TMP_FILE"

mv "$TMP_FILE" "$OUTPUT_PATH"
trap - EXIT

echo "Rendered Cloudflare real IP config: $OUTPUT_PATH"
