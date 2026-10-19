#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPLATE_PATH="${2:-$ROOT_DIR/deploy/templates/nginx/ideaboard.conf.template}"
OUTPUT_PATH="${1:-}"

if [[ -z "$OUTPUT_PATH" ]]; then
  echo "Usage: render-nginx-template.sh <output-path> [template-path]" >&2
  exit 1
fi

NGINX_API_UPSTREAM="${NGINX_API_UPSTREAM:-http://127.0.0.1:6000}"
NGINX_AUTH_UPSTREAM="${NGINX_AUTH_UPSTREAM:-http://127.0.0.1:6001}"
NGINX_AUTH_HEALTH_UPSTREAM="${NGINX_AUTH_HEALTH_UPSTREAM:-http://127.0.0.1:6001/api/health}"
NGINX_REALTIME_UPSTREAM="${NGINX_REALTIME_UPSTREAM:-http://127.0.0.1:6002}"
NGINX_WEB_ROOT="${NGINX_WEB_ROOT:-/usr/share/nginx/html}"
NGINX_ROOT_DIRECTIVE="${NGINX_ROOT_DIRECTIVE:-try_files \$uri \$uri/ /index.html}"
NGINX_REAL_IP_INCLUDE="${NGINX_REAL_IP_INCLUDE:-}"
NGINX_SERVER_NAME="${NGINX_SERVER_NAME:-_}"
NGINX_LEGACY_API_SERVER_NAME="${NGINX_LEGACY_API_SERVER_NAME:-api.ideaboard.art}"

for server_name_var in NGINX_SERVER_NAME NGINX_LEGACY_API_SERVER_NAME; do
  server_name_value="${!server_name_var}"
  if [[ "$server_name_value" == *";"* || "$server_name_value" == *$'\n'* || "$server_name_value" == *$'\r'* ]]; then
    echo "$server_name_var must not contain semicolons or newlines." >&2
    exit 1
  fi
done

if [[ "$NGINX_SERVER_NAME" == "$NGINX_LEGACY_API_SERVER_NAME" ]]; then
  echo "NGINX_SERVER_NAME and NGINX_LEGACY_API_SERVER_NAME must be different." >&2
  exit 1
fi

if [[ -n "$NGINX_REAL_IP_INCLUDE" ]]; then
  if [[ "$NGINX_REAL_IP_INCLUDE" != /* || "$NGINX_REAL_IP_INCLUDE" =~ [[:space:]\;] ]]; then
    echo "NGINX_REAL_IP_INCLUDE must be an absolute path without whitespace or semicolons." >&2
    exit 1
  fi
  NGINX_REAL_IP_INCLUDE_DIRECTIVE="include $NGINX_REAL_IP_INCLUDE;"
else
  NGINX_REAL_IP_INCLUDE_DIRECTIVE="# Real client IP restoration disabled; set NGINX_REAL_IP_INCLUDE after restricting origin access to trusted proxies."
fi

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[|&\\]/\\&/g'
}

OUTPUT_DIR="$(dirname "$OUTPUT_PATH")"
mkdir -p "$OUTPUT_DIR"
TMP_OUTPUT="$(mktemp "$OUTPUT_DIR/.ideaboard-nginx.XXXXXX")"
cleanup_path "$TMP_OUTPUT"

sed \
  -e "s|\${NGINX_SERVER_NAME}|$(escape_sed_replacement "$NGINX_SERVER_NAME")|g" \
  -e "s|\${NGINX_LEGACY_API_SERVER_NAME}|$(escape_sed_replacement "$NGINX_LEGACY_API_SERVER_NAME")|g" \
  -e "s|\${NGINX_API_UPSTREAM}|$(escape_sed_replacement "$NGINX_API_UPSTREAM")|g" \
  -e "s|\${NGINX_AUTH_UPSTREAM}|$(escape_sed_replacement "$NGINX_AUTH_UPSTREAM")|g" \
  -e "s|\${NGINX_AUTH_HEALTH_UPSTREAM}|$(escape_sed_replacement "$NGINX_AUTH_HEALTH_UPSTREAM")|g" \
  -e "s|\${NGINX_REALTIME_UPSTREAM}|$(escape_sed_replacement "$NGINX_REALTIME_UPSTREAM")|g" \
  -e "s|\${NGINX_WEB_ROOT}|$(escape_sed_replacement "$NGINX_WEB_ROOT")|g" \
  -e "s|\${NGINX_ROOT_DIRECTIVE}|$(escape_sed_replacement "$NGINX_ROOT_DIRECTIVE")|g" \
  -e "s|\${NGINX_REAL_IP_INCLUDE_DIRECTIVE}|$(escape_sed_replacement "$NGINX_REAL_IP_INCLUDE_DIRECTIVE")|g" \
  "$TEMPLATE_PATH" > "$TMP_OUTPUT"

mv "$TMP_OUTPUT" "$OUTPUT_PATH"
trap - EXIT
