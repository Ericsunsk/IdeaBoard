#!/usr/bin/env bash
# Render-nginx-template contract: every business-route location must emit
# cache-bypass headers so authenticated, SSE, and write traffic can never be
# cached by an intermediary in front of the same-origin gateway.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
template="${repo_root}/deploy/templates/nginx/ideaboard.conf.template"
output="$(mktemp)"
legacy_host="legacy-api.example.test"

cleanup() {
  rm -f "$output"
}
trap cleanup EXIT

if [[ ! -f "$template" ]]; then
  printf 'render-nginx-template-test: missing template %s\n' "$template" >&2
  exit 2
fi

NGINX_SERVER_NAME="ideaboard.art" \
  NGINX_LEGACY_API_SERVER_NAME="$legacy_host" \
  NGINX_API_UPSTREAM="http://127.0.0.1:6000" \
  NGINX_AUTH_UPSTREAM="http://127.0.0.1:6001" \
  NGINX_AUTH_HEALTH_UPSTREAM="http://127.0.0.1:6001/api/health" \
  NGINX_WEB_ROOT="/usr/share/nginx/html" \
  NGINX_ROOT_DIRECTIVE="try_files \$uri \$uri/ /index.html" \
  bash "${script_dir}/render-nginx-template.sh" "$output" >/dev/null

# Every business location must add Cache-Control: no-store and Pragma: no-cache
# so intermediaries never cache authenticated or state-changing responses.
expected_locations=(
  "location = /api/health"
  "location = /api/auth/health"
  "location /api/auth/"
  "location /api/user/tasks/"
  "location = /api/user/events"
  "location /api/"
  "location /uploads/"
)

if ! grep -qF "server_name $legacy_host;" "$output" || ! grep -qE '^[[:space:]]*return 444;' "$output"; then
  printf 'render-nginx-template-test: legacy API host rejection is missing\n' >&2
  exit 2
fi

for needle in "${expected_locations[@]}"; do
  if ! grep -qF "$needle" "$output"; then
    printf 'render-nginx-template-test: rendered config missing %s\n' "$needle" >&2
    exit 2
  fi
done

# Cache-Control: no-store,no-cache,must-revalidate must appear in every business
# location. Count expected and actual occurrences to ensure one per location.
expected_count=${#expected_locations[@]}
actual_count="$(grep -cE 'add_header Cache-Control "no-store, no-cache, must-revalidate" always' "$output" || true)"
if (( actual_count != expected_count )); then
  printf 'render-nginx-template-test: Cache-Control count = %d, want %d\n' "$actual_count" "$expected_count" >&2
  exit 2
fi

pragma_count="$(grep -cE 'add_header Pragma "no-cache" always' "$output" || true)"
if (( pragma_count != expected_count )); then
  printf 'render-nginx-template-test: Pragma count = %d, want %d\n' "$pragma_count" "$expected_count" >&2
  exit 2
fi

# SSE routes must disable buffering so the gateway can stream events.
if [[ "$(grep -cE '^[[:space:]]*proxy_buffering off' "$output" || true)" -lt 2 ]]; then
  printf 'render-nginx-template-test: SSE route must set proxy_buffering off\n' >&2
  exit 2
fi

if NGINX_SERVER_NAME="api.ideaboard.art" \
  NGINX_LEGACY_API_SERVER_NAME="api.ideaboard.art" \
  bash "${script_dir}/render-nginx-template.sh" "$output" >/dev/null 2>&1; then
  printf 'render-nginx-template-test: identical primary and legacy hosts must fail closed\n' >&2
  exit 2
fi
if [[ "$(grep -cE 'add_header X-Accel-Buffering "no" always' "$output" || true)" -lt 2 ]]; then
  printf 'render-nginx-template-test: SSE route must set X-Accel-Buffering: no\n' >&2
  exit 2
fi

# Forwarded protocol is derived from the actual connection, never a client
# supplied X-Forwarded-Proto value.
if grep -qF '$http_x_forwarded_proto' "$output" || grep -qF '$proxy_x_forwarded_proto' "$output"; then
  printf 'render-nginx-template-test: rendered config trusts client X-Forwarded-Proto\n' >&2
  exit 2
fi
forwarded_proto_count="$(grep -cE 'proxy_set_header X-Forwarded-Proto \$scheme;' "$output" || true)"
if (( forwarded_proto_count != 7 )); then
  printf 'render-nginx-template-test: X-Forwarded-Proto count = %d, want 7\n' "$forwarded_proto_count" >&2
  exit 2
fi

# Multipart uploads require a generous client body size; ensure it's preserved.
if ! grep -qE '^[[:space:]]*client_max_body_size 50m' "$output"; then
  printf 'render-nginx-template-test: client_max_body_size 50m missing\n' >&2
  exit 2
fi

printf 'render-nginx-template-test: ok (%d business locations, all cache-bypass headers present)\n' "$expected_count"
