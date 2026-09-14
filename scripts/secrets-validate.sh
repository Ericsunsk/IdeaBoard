#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  echo "Usage: $0 <decrypted-secrets.json>" >&2
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

bundle_file="$1"
[[ -f "$bundle_file" ]] || { echo "secret bundle does not exist: $bundle_file" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

required_keys=(
  POSTGRES_PASSWORD
  API_DB_PASSWORD
  AUTH_DB_PASSWORD
  AUTH_INTERNAL_KEY
  APP_SECRETS_ENCRYPTION_KEY
  CREDITS_WELCOME_IDENTITY_KEY
  REWARDS_CODE_HMAC_KEYS
  BETTER_AUTH_SECRET
  NATS_PASSWORD
  IMGPROXY_KEY
  IMGPROXY_SALT
)

decode_base64() {
  if base64 --decode </dev/null >/dev/null 2>&1; then
    base64 --decode
  else
    base64 -D
  fi
}

jq -e 'type == "object"' "$bundle_file" >/dev/null || {
  echo "secret bundle must be a JSON object" >&2
  exit 1
}

for key in "${required_keys[@]}"; do
  jq -e --arg key "$key" 'has($key) and (.[$key] | type == "string" and length > 0)' "$bundle_file" >/dev/null || {
    echo "missing or empty secret: $key" >&2
    exit 1
  }
done

for key in POSTGRES_PASSWORD API_DB_PASSWORD AUTH_DB_PASSWORD AUTH_INTERNAL_KEY BETTER_AUTH_SECRET; do
  length="$(jq -r --arg key "$key" '.[$key] | length' "$bundle_file")"
  (( length >= 32 )) || { echo "$key must be at least 32 characters" >&2; exit 1; }
done

for key in IMGPROXY_KEY IMGPROXY_SALT; do
  value="$(jq -r --arg key "$key" '.[$key]' "$bundle_file")"
  [[ "$value" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "$key must be 64 hex characters" >&2; exit 1; }
done

nats_password="$(jq -r '.NATS_PASSWORD' "$bundle_file")"
[[ "$nats_password" =~ ^[A-Za-z0-9._~-]{32,}$ ]] || {
  echo "NATS_PASSWORD must use URL-safe characters and be at least 32 characters" >&2
  exit 1
}

for key in APP_SECRETS_ENCRYPTION_KEY CREDITS_WELCOME_IDENTITY_KEY; do
  value="$(jq -r --arg key "$key" '.[$key]' "$bundle_file")"
  decoded_length="$(printf '%s' "$value" | decode_base64 2>/dev/null | wc -c | tr -d ' ')"
  [[ "$decoded_length" == "32" ]] || {
    echo "$key must be base64 that decodes to exactly 32 bytes" >&2
    exit 1
  }
done

jq -e '.REWARDS_CODE_HMAC_KEYS | fromjson | type == "object"' "$bundle_file" >/dev/null || {
  echo "REWARDS_CODE_HMAC_KEYS must contain a JSON object" >&2
  exit 1
}

if [[ "$(jq -r '.IMGPROXY_KEY' "$bundle_file")" == "$(jq -r '.IMGPROXY_SALT' "$bundle_file")" ]]; then
  echo "IMGPROXY_KEY and IMGPROXY_SALT must be different" >&2
  exit 1
fi

echo "secret bundle is valid: $bundle_file"
