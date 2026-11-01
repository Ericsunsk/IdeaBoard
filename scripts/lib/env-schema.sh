#!/usr/bin/env bash
# Canonical key extraction and non-app contracts for the offline env gates.

# --- schema extraction (newline-separated key lists) --------------------------

# Go env names from `env:"NAME"` struct tags.
go_schema() { grep -o 'env:"[A-Z][A-Z0-9_]*"' "$1" | cut -d'"' -f2 | sort -u || true; }

# Go serving-required names from ValidateRequired's missing list.
go_required() { grep -o 'missing = append(missing, "[A-Z][A-Z0-9_]*")' "$1" | cut -d'"' -f2 | sort -u || true; }

# Node env names from getEnv/getEnvInt(source, 'NAME') reads, plus the
# production switch inputs read directly (NODE_ENV/APP_ENV).
node_schema() {
  {
    grep -oE "getEnv(Int)?\(source, '[A-Z][A-Z0-9_]*'" "$1" | cut -d"'" -f2
    printf 'NODE_ENV\nAPP_ENV\n'
  } | sort -u
}

# Node always-required names from problems.push('NAME is required').
node_required() { grep -o "problems\.push('[A-Z][A-Z0-9_]* is required'" "$1" | cut -d"'" -f2 | awk '{print $1}' | sort -u || true; }

# Worker bindings come from its owned Env interface, not provider payloads.
worker_schema() {
  sed -n '/^export interface Env {$/,/^}/p' "$1" |
    sed -nE 's/^[[:space:]]+([A-Z][A-Z0-9_]*): string;$/\1/p' | sort -u
}

# Worker bindings policy. Env remains the only key-name authority; the API
# owns discovery plus the shared Worker credential, while API-worker receives
# only Worker credentials. Unrelated consumers stay on the root namespace.
worker_binding_source_path() { printf '/ai-proxy\n'; }

worker_binding_keys() {
  local schema="$1" consumer="$2"
  case "$consumer" in
    worker | api) worker_schema "$schema" ;;
    api-worker) worker_schema "$schema" | grep '^WORKER_' || true ;;
    *) return 0 ;;
  esac
}

worker_binding_paths() {
  case "$1" in
    worker) worker_binding_source_path ;;
    api | api-worker) printf '/\n'; worker_binding_source_path ;;
    *) printf '/\n' ;;
  esac
}

worker_binding_hash() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    shasum -a 256 | awk '{print $1}'
  fi
}

# Local migration tolerates matching root copies until operators remove them.
# Compare raw dotenv lines by hash; do not source or reinterpret token values.
worker_binding_root_copies_match() {
  local schema="$1" root_export="$2" worker_export="$3" key root_lines want got
  while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    root_lines="$(grep -E "^${key}=" "$root_export" || true)"
    [[ -n "$root_lines" ]] || continue
    want="$(grep -E "^${key}=" "$worker_export" | worker_binding_hash)"
    got="$(printf '%s\n' "$root_lines" | worker_binding_hash)"
    if [[ "$want" != "$got" ]]; then
      printf 'error: root / 的 %s 与 %s 不一致（root 副本待用户在 dashboard 手动删除）\n' "$key" "$(worker_binding_source_path)" >&2
      return 1
    fi
  done < <(worker_binding_keys "$schema" worker)
}

# Validate the checked-in startup adapter against the source policy. Read only
# command declarations, never local env files or actual Infisical values.
worker_binding_check_startup() {
  local file="$1" consumer command paths expected raw_count parsed_count found_api=0
  while IFS=$'\t' read -r consumer command; do
    [[ "$command" == *infisical-run.sh* ]] || continue
    [[ "$consumer" != api ]] || found_api=1
    # Only inspect wrapper flags before its -- delimiter. This is a static
    # declaration gate, not a shell parser: accept the documented unquoted
    # --path /literal form and reject alternate/dynamic spellings explicitly.
    command="${command%% -- *}"
    raw_count="$(grep -o -- '--path' <<<"$command" | wc -l | tr -d ' ' || true)"
    paths="$(grep -oE -- '--path /[A-Za-z0-9/_-]*([[:space:]]|$)' <<<"$command" | awk '{print $2}' || true)"
    parsed_count="$(printf '%s\n' "$paths" | grep -c '^/' || true)"
    if [[ "$raw_count" != "$parsed_count" ]]; then
      printf 'Worker bindings: unsupported startup --path spelling for %s (use unquoted --path /literal)\n' "$consumer" >&2
      return 1
    fi
    [[ -n "$paths" ]] || paths=/
    expected="$(worker_binding_paths "$consumer")"
    if [[ "$paths" != "$expected" ]]; then
      printf 'Worker bindings: startup paths for %s differ from source policy\n' "$consumer" >&2
      return 1
    fi
  done < <(awk '
    /^  [a-z][a-z0-9-]*:$/ { name=$1; sub(/:$/, "", name) }
    /command:/ { print name "\t" $0 }
  ' "$file")
  if ((found_api == 0)); then
    printf 'Worker bindings: missing Infisical startup command for api\n' >&2
    return 1
  fi
}

# Consume actual `docker compose config --format json`. Validate every service,
# so a binding cannot silently leak into auth/realtime/other containers.
worker_binding_check_compose() {
  local schema="$1" all api worker
  all="$(worker_binding_keys "$schema" worker | jq -Rsc 'split("\n") | map(select(length > 0))')"
  api="$(worker_binding_keys "$schema" api | jq -Rsc 'split("\n") | map(select(length > 0))')"
  worker="$(worker_binding_keys "$schema" api-worker | jq -Rsc 'split("\n") | map(select(length > 0))')"
  jq -e --argjson all "$all" --argjson api "$api" --argjson worker "$worker" '
    .services as $services |
    ($services | has("api") and has("api-worker")) and
    ([ $services | to_entries[] |
      .key as $name |
      ((.value.environment // {} | keys) - ((.value.environment // {} | keys) - $all) | sort) as $actual |
      (if $name == "api" then $api elif $name == "api-worker" then $worker else [] end | sort) as $expected |
      $actual == $expected
    ] | all)
  ' >/dev/null
}

# Example keys (active or commented) from KEY= / # KEY= lines.
example_keys() { grep -oE '^[[:space:]]*#?[[:space:]]*[A-Z][A-Z0-9_]*=' "$1" | grep -oE '[A-Z][A-Z0-9_]*' | sort -u || true; }

# --- allowlists ---------------------------------------------------------------

# Deploy-only docker example keys with no app schema (release images, reverse
# proxy, cosign, backup, host port overrides). One entry per key, sorted.
deploy_allowlist() {
  cat <<'EOF'
API_HOST_PORT
API_METRICS_HOST_PORT
AUTH_HOST_PORT
BACKUP_ENCRYPTION_RECIPIENT
COSIGN_CERTIFICATE_IDENTITY_REGEXP
COSIGN_CERTIFICATE_OIDC_ISSUER
COSIGN_VERIFY
IDEABOARD_API_IMAGE
IDEABOARD_API_MIGRATE_IMAGE
IDEABOARD_AUTH_IMAGE
IDEABOARD_AUTH_MIGRATE_IMAGE
IDEABOARD_COMPOSE_OVERRIDE_FILE
IDEABOARD_REALTIME_IMAGE
NGINX_API_UPSTREAM
NGINX_AUTH_HEALTH_UPSTREAM
NGINX_AUTH_UPSTREAM
NGINX_REALTIME_UPSTREAM
NGINX_REAL_IP_INCLUDE
NGINX_SERVER_NAME
NATS_CLIENT_PORT
NATS_MONITOR_PORT
NPM_NETWORK_NAME
RT_HOST_PORT
EOF
}

# Third-party image contracts (never renamed): postgres server vars and the
# imgproxy server surface. Runtime secrets shared with the app (IMGPROXY_KEY /
# IMGPROXY_SALT, NATS_PASSWORD) stay in app schema as well.
third_party_allowlist() {
  cat <<'EOF'
IMGPROXY_ALLOW_LINK_LOCAL_SOURCE_ADDRESSES
IMGPROXY_ALLOW_LOOPBACK_SOURCE_ADDRESSES
IMGPROXY_ALLOW_PRIVATE_SOURCE_ADDRESSES
IMGPROXY_ALLOWED_SOURCES
IMGPROXY_DOWNLOAD_TIMEOUT
IMGPROXY_LOCAL_FILESYSTEM_ROOT
IMGPROXY_MAX_REDIRECTS
IMGPROXY_MAX_SRC_FILE_SIZE
IMGPROXY_MAX_SRC_RESOLUTION
IMGPROXY_S3_ACCESS_KEY_ID
IMGPROXY_S3_ALLOWED_BUCKETS
IMGPROXY_S3_ENDPOINT
IMGPROXY_S3_REGION
IMGPROXY_S3_SECRET_ACCESS_KEY
IMGPROXY_TTL
IMGPROXY_USE_S3
POSTGRES_DB
POSTGRES_PASSWORD
POSTGRES_USER
EOF
}

# Direct process-env reads outside the typed schema (healthcheck override,
# dotenv path selection, schemarelease/atlas tool-binary override for CI and
# local runs; never a secret, never stored in Infisical). Kept minimal by
# design.
command_allowlist() {
  cat <<'EOF'
ATLAS_BINARY
ENV_FILE
HEALTHCHECK_URL
EOF
}

contains_key() {
  local needle="$1"
  local list="$2"
  grep -Fxq "$needle" <<<"$list"
}
