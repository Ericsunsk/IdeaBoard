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
    if [[ -f "$dir/docker/compose.yml" ]]; then
      cd "$dir" && pwd
      return
    fi
  done

  echo "Error: cannot find stack root. Set IDEABOARD_STACK_DIR=/opt/stacks/ideaboard." >&2
  exit 1
}

ROOT_DIR="$(find_stack_root)"
COMPOSE_FILE="$ROOT_DIR/docker/compose.yml"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/docker/.env}"
ENV_EXAMPLE="$ROOT_DIR/docker/.env.example"
BACKUP_DIR="${BACKUP_DIR:-$ROOT_DIR/backups/postgres}"
BACKUP_RETAIN="${BACKUP_RETAIN:-5}"

command="${1:-deploy}"
if [[ "$command" == "update" ]]; then
  command="deploy"
fi

if [[ "$command" == "delete" || "$command" == "uninstall" ]]; then
  shift || true
  IDEABOARD_STACK_DIR="$ROOT_DIR" exec "$ROOT_DIR/scripts/uninstall-vps.sh" "$@"
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

  for key in POSTGRES_PASSWORD AUTH_INTERNAL_KEY BETTER_AUTH_SECRET; do
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
    APP_BASE_URL \
    API_BASE_URL \
    CLOUDFLARE_TUNNEL_TOKEN \
    POSTGRES_PASSWORD \
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

ensure_image_namespace_for_deploy() {
  local namespace
  namespace="$(env_value IMAGE_NAMESPACE)"
  if [[ "$namespace" == "your-github-owner/your-repo" || -z "$namespace" ]]; then
    namespace="$(infer_image_namespace)"
    if [[ -n "$namespace" ]]; then
      set_env_value IMAGE_NAMESPACE "$namespace"
      echo "Inferred IMAGE_NAMESPACE=$namespace"
      return
    fi
    die "cannot infer IMAGE_NAMESPACE. Set IMAGE_NAMESPACE in $ENV_FILE"
  fi
}

docker_compose() {
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
}

wait_for_postgres() {
  local timeout="${POSTGRES_WAIT_TIMEOUT_SECONDS:-120}"
  local elapsed=0

  echo "Waiting for PostgreSQL to become ready..."
  while (( elapsed < timeout )); do
    if docker_compose exec -T postgres sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" -h 127.0.0.1' >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done

  docker_compose logs --tail=120 postgres >&2 || true
  die "PostgreSQL did not become ready within ${timeout}s"
}

prune_postgres_backups() {
  local retain="$BACKUP_RETAIN"
  local backups=()
  local backup
  local delete_count
  local i

  [[ "$retain" =~ ^[0-9]+$ ]] || retain=5
  (( retain > 0 )) || retain=5

  while IFS= read -r backup; do
    backups+=("$backup")
  done < <(find "$BACKUP_DIR" -type f -name 'postgres-*.sql' | sort)

  delete_count=$(( ${#backups[@]} - retain ))
  if (( delete_count <= 0 )); then
    return
  fi

  for (( i = 0; i < delete_count; i++ )); do
    rm -f -- "${backups[$i]}"
  done
}

backup_postgres() {
  local timestamp
  local database
  local backup_file
  local tmp_file

  mkdir -p "$BACKUP_DIR"
  timestamp="$(date +%Y%m%d-%H%M%S)"
  database="$(env_value POSTGRES_DB)"
  database="${database:-ideaboard}"
  backup_file="$BACKUP_DIR/postgres-${database}-${timestamp}.sql"
  tmp_file="${backup_file}.tmp"

  echo "Creating PostgreSQL backup: $backup_file"
  if docker_compose exec -T postgres sh -c 'pg_dump --clean --if-exists --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$tmp_file"; then
    if ! mv "$tmp_file" "$backup_file"; then
      rm -f -- "$tmp_file"
      die "PostgreSQL backup finalize failed; deployment aborted before migration"
    fi
    prune_postgres_backups
    echo "PostgreSQL backup completed: $backup_file"
    return 0
  fi

  rm -f -- "$tmp_file"
  die "PostgreSQL backup failed; deployment aborted before migration"
}

print_backend_health_logs() {
  echo "Recent service logs for health gate failure:" >&2
  docker_compose ps >&2 || true
  docker_compose logs --tail=120 gateway api auth postgres nats >&2 || true
}

wait_for_nats() {
  local timeout="${NATS_WAIT_TIMEOUT_SECONDS:-120}"
  local elapsed=0

  echo "Waiting for NATS JetStream to become ready..."
  while (( elapsed < timeout )); do
    if docker_compose exec -T nats wget -q -T 3 -O /dev/null 'http://127.0.0.1:8222/healthz?js-enabled-only=true' >/dev/null 2>&1; then
      echo "NATS JetStream health is OK"
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done

  print_backend_health_logs
  die "NATS JetStream did not become ready within ${timeout}s"
}

wait_for_http_from_gateway() {
  local name="$1"
  local url="$2"
  local timeout="${HEALTH_WAIT_TIMEOUT_SECONDS:-120}"
  local elapsed=0

  echo "Waiting for $name health: $url"
  while (( elapsed < timeout )); do
    if docker_compose exec -T gateway sh -c 'wget -q -T 3 -O /dev/null "$1"' sh "$url" >/dev/null 2>&1; then
      echo "$name health is OK"
      return 0
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done

  print_backend_health_logs
  die "$name health gate failed after ${timeout}s: $url"
}

wait_for_backend_health() {
  wait_for_http_from_gateway "api" "http://api:6000/api/health"
  wait_for_http_from_gateway "auth" "http://auth:6001/api/health"
}

is_false_value() {
  local value
  value="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  [[ "$value" == "0" || "$value" == "false" || "$value" == "no" || "$value" == "off" ]]
}

event_bus_enabled() {
  local value
  value="$(env_value NATS_EVENT_BUS_ENABLED)"
  if [[ -z "$value" ]]; then
    value="true"
  fi
  ! is_false_value "$value"
}

check_river_schema() {
  local result

  echo "Checking River schema..."
  result="$(
    docker_compose exec -T postgres sh -c \
      'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT CASE WHEN to_regclass('\''public.river_job'\'') IS NULL THEN '\''missing'\'' ELSE '\''ok'\'' END"'
  )" || {
    print_backend_health_logs
    die "River schema check failed"
  }
  if [[ "$(printf '%s' "$result" | tr -d '[:space:]')" != "ok" ]]; then
    print_backend_health_logs
    die "River schema is missing river_job"
  fi
  echo "River schema is OK"
}

check_nats_event_stream() {
  local stream_name
  local body

  if ! event_bus_enabled; then
    echo "Skipping NATS stream check because NATS_EVENT_BUS_ENABLED=false"
    return 0
  fi

  stream_name="$(env_value NATS_STREAM_NAME)"
  stream_name="${stream_name:-IDEABOARD_EVENTS}"

  echo "Checking NATS JetStream stream: $stream_name"
  body="$(
    docker_compose exec -T nats wget -q -T 3 -O - 'http://127.0.0.1:8222/jsz?streams=true&consumers=true&config=true'
  )" || {
    print_backend_health_logs
    die "NATS JetStream stream check failed"
  }
  if ! printf '%s' "$body" | grep -F "\"$stream_name\"" >/dev/null; then
    print_backend_health_logs
    die "NATS JetStream stream not found: $stream_name"
  fi
  echo "NATS JetStream stream is OK"
}

run_backend_smoke_checks() {
  wait_for_postgres
  wait_for_nats
  wait_for_backend_health
  check_river_schema
  check_nats_event_stream
  echo "Backend smoke checks passed"
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
ensure_image_namespace_for_deploy
IMAGE_NAMESPACE="$(env_value IMAGE_NAMESPACE)"

case "$command" in
  deploy)
    login_ghcr_if_configured
    docker_compose pull
    docker_compose up -d postgres
    wait_for_postgres
    backup_postgres
    docker_compose run --rm auth npm run migrate:dist
    docker_compose up -d
    run_backend_smoke_checks
    docker_compose ps
    ;;
  smoke)
    run_backend_smoke_checks
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
Usage: ideaboard [update|smoke|restart|stop|status|logs|delete|uninstall|init-env|pull|up|migrate|ps]

Default command: update
Environment file: $ENV_FILE
EOF
    exit 1
    ;;
esac
