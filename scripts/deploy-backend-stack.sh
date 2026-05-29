#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

find_stack_root() {
  local dir

  if [[ -n "${IDEABOARD_STACK_DIR:-}" ]]; then
    cd "$IDEABOARD_STACK_DIR" && pwd
    return
  fi

  for dir in \
    "$SCRIPT_DIR/.." \
    "$PWD" \
    "/opt/stacks/ideaboard" \
    "/opt/ideaboard"; do
    if [[ -f "$dir/docker/compose.prod.yml" ]]; then
      cd "$dir" && pwd
      return
    fi
  done

  echo "Error: cannot find stack root. Set IDEABOARD_STACK_DIR=/opt/stacks/ideaboard." >&2
  exit 1
}

ROOT_DIR="$(find_stack_root)"
COMPOSE_FILE="$ROOT_DIR/docker/compose.prod.yml"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/docker/.env.production}"
ENV_EXAMPLE="$ROOT_DIR/docker/.env.production.example"

command="${1:-deploy}"
if [[ "$command" == "update" ]]; then
  command="deploy"
fi

die() {
  echo "Error: $*" >&2
  exit 1
}

generate_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32
    return
  fi
  dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d '\n'
  echo
}

env_value() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$ENV_FILE"
}

set_env_value() {
  local key="$1"
  local value="$2"
  local tmp
  tmp="$(mktemp)"
  awk -v key="$key" -v value="$value" '
    BEGIN { found = 0 }
    $0 ~ "^" key "=" {
      print key "=" value
      found = 1
      next
    }
    { print }
    END {
      if (!found) print key "=" value
    }
  ' "$ENV_FILE" > "$tmp"
  mv "$tmp" "$ENV_FILE"
}

is_placeholder() {
  local value="$1"
  [[ -z "$value" || "$value" == replace-with-* || "$value" == "your-github-owner/your-repo" || "$value" == *"example.com"* ]]
}

infer_image_namespace() {
  local remote
  remote="$(git -C "$ROOT_DIR" config --get remote.origin.url 2>/dev/null || true)"
  if [[ "$remote" =~ github.com[:/]([^/]+)/([^/.]+)(\.git)?$ ]]; then
    local owner="${BASH_REMATCH[1]}"
    local repo="${BASH_REMATCH[2]}"
    printf '%s/%s\n' \
      "$(printf '%s' "$owner" | tr '[:upper:]' '[:lower:]')" \
      "$(printf '%s' "$repo" | tr '[:upper:]' '[:lower:]')"
  fi
}

ensure_env_file() {
  if [[ ! -f "$ENV_FILE" ]]; then
    [[ -f "$ENV_EXAMPLE" ]] || die "missing $ENV_EXAMPLE"
    cp "$ENV_EXAMPLE" "$ENV_FILE"
    echo "Created $ENV_FILE"
  fi

  local namespace
  namespace="$(env_value IMAGE_NAMESPACE)"
  if [[ "$namespace" == "your-github-owner/your-repo" || -z "$namespace" ]]; then
    namespace="$(infer_image_namespace)"
    if [[ -n "$namespace" ]]; then
      set_env_value IMAGE_NAMESPACE "$namespace"
      echo "Inferred IMAGE_NAMESPACE=$namespace"
    fi
  fi

  for key in MYSQL_ROOT_PASSWORD MYSQL_PASSWORD AUTH_INTERNAL_KEY BETTER_AUTH_SECRET; do
    if is_placeholder "$(env_value "$key")"; then
      set_env_value "$key" "$(generate_secret)"
      echo "Generated $key"
    fi
  done
}

validate_env_file() {
  local missing=()
  local key

  for key in \
    IMAGE_NAMESPACE \
    APP_PUBLIC_BASE_URL \
    APP_PUBLIC_ASSET_BASE_URL \
    APP_CORS_ALLOWED_ORIGINS \
    AUTH_TRUSTED_ORIGINS \
    BETTER_AUTH_URL \
    MYSQL_ROOT_PASSWORD \
    MYSQL_PASSWORD \
    AUTH_INTERNAL_KEY \
    BETTER_AUTH_SECRET; do
    if is_placeholder "$(env_value "$key")"; then
      missing+=("$key")
    fi
  done

  if (( ${#missing[@]} > 0 )); then
    echo "Please edit $ENV_FILE and replace these values:" >&2
    printf '  - %s\n' "${missing[@]}" >&2
    return 1
  fi
  return 0
}

docker_compose() {
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
}

login_ghcr_if_configured() {
  if [[ -z "${GHCR_TOKEN:-}" ]]; then
    return
  fi

  local username
  username="${GHCR_USERNAME:-${IMAGE_NAMESPACE%%/*}}"
  [[ -n "$username" ]] || die "GHCR_TOKEN is set, but GHCR_USERNAME or IMAGE_NAMESPACE is empty"

  printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u "$username" --password-stdin
}

ensure_env_file

if [[ "$command" == "init-env" ]]; then
  if validate_env_file; then
    echo "Environment file is ready: $ENV_FILE"
  else
    echo "Environment file was created or updated: $ENV_FILE"
  fi
  exit 0
fi

validate_env_file || exit 1
IMAGE_NAMESPACE="$(env_value IMAGE_NAMESPACE)"

case "$command" in
  deploy)
    login_ghcr_if_configured
    docker_compose pull
    docker_compose up -d mysql redis
    docker_compose run --rm auth npm run migrate:dist
    docker_compose up -d
    docker_compose ps
    ;;
  restart)
    docker_compose restart
    docker_compose ps
    ;;
  stop)
    docker_compose stop
    ;;
  status)
    docker_compose ps
    ;;
  logs)
    shift || true
    docker_compose logs -f "$@"
    ;;
  pull)
    login_ghcr_if_configured
    docker_compose pull
    ;;
  up)
    docker_compose up -d
    ;;
  migrate)
    docker_compose run --rm auth npm run migrate:dist
    ;;
  ps)
    docker_compose ps
    ;;
  *)
    cat >&2 <<EOF
Usage: ideaboard [update|restart|stop|status|logs|init-env|pull|up|migrate|ps]

Default command: update
Environment file: $ENV_FILE
EOF
    exit 1
    ;;
esac
