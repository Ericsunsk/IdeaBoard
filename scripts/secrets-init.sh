#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  echo "Usage: $0 <sops-output.json>" >&2
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

output_file="$1"
command -v sops >/dev/null 2>&1 || { echo "sops is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
command -v openssl >/dev/null 2>&1 || { echo "openssl is required" >&2; exit 1; }

recipient="${SOPS_AGE_RECIPIENT:-}"
[[ -n "$recipient" ]] || { echo "SOPS_AGE_RECIPIENT is required" >&2; exit 1; }

work_dir="$(mktemp -d)"
cleanup() { rm -rf "$work_dir"; }
trap cleanup EXIT

random_hex() { openssl rand -hex 32; }
random_base64() { openssl rand -base64 32 | tr -d '\n'; }

plain_file="$work_dir/secrets.json"
encrypted_file="$work_dir/secrets.sops.json"
jq -n \
  --arg postgres "$(random_hex)" \
  --arg apiDb "$(random_hex)" \
  --arg authDb "$(random_hex)" \
  --arg internal "$(random_hex)" \
  --arg encryption "$(random_base64)" \
  --arg credits "$(random_base64)" \
  --arg rewards "$(random_base64)" \
  --arg authSecret "$(random_hex)" \
  --arg nats "$(random_hex)" \
  --arg imgKey "$(random_hex)" \
  --arg imgSalt "$(random_hex)" \
  '{
    POSTGRES_PASSWORD: $postgres,
    API_DB_PASSWORD: $apiDb,
    AUTH_DB_PASSWORD: $authDb,
    AUTH_INTERNAL_KEY: $internal,
    APP_SECRETS_ENCRYPTION_KEY: $encryption,
    CREDITS_WELCOME_IDENTITY_KEY: $credits,
    REWARDS_CODE_HMAC_KEYS: ({current: {version: "v1", key: $rewards}, previous: null} | tojson),
    BETTER_AUTH_SECRET: $authSecret,
    NATS_PASSWORD: $nats,
    IMGPROXY_KEY: $imgKey,
    IMGPROXY_SALT: $imgSalt
  }' > "$plain_file"

bash "$(dirname "$0")/secrets-validate.sh" "$plain_file" >/dev/null
sops --encrypt --age "$recipient" "$plain_file" > "$encrypted_file"
mkdir -p "$(dirname "$output_file")"
mv "$encrypted_file" "$output_file"
chmod 600 "$output_file"
echo "created encrypted secret bundle: $output_file"
