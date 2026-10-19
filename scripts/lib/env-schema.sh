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
