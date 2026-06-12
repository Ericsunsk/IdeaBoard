#!/usr/bin/env bash

set -euo pipefail
umask 077

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
BACKUP_DIR="${BACKUP_DIR:-$(cd "$ROOT_DIR/.." && pwd)/$(basename "$ROOT_DIR")-backups}"
BACKUP_RETAIN="${BACKUP_RETAIN:-5}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-ideaboard}"

command="${1:-deploy}"
is_update=0
if [[ "$command" == "update" ]]; then
  command="deploy"
  is_update=1
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

generate_url_safe_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  echo
}

env_value() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$ENV_FILE"
}

env_or_file_value() {
  local key="$1"
  local value="${!key:-}"
  if [[ -z "$value" ]]; then
    value="$(env_value "$key")"
  fi
  printf '%s\n' "$value"
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

is_ideaboard_image_ref() {
  local value="$1"
  local component="$2"
  [[ "$value" =~ ^ghcr\.io/[^[:space:]/]+/[^[:space:]/]+/ideaboard-${component}@sha256:[a-fA-F0-9]{64}$ ]]
}

is_url_userinfo_value() {
  local value="$1"
  [[ "$value" =~ ^[A-Za-z0-9._~-]+$ ]]
}

api_host_port() {
  local value
  value="$(env_or_file_value API_HOST_PORT)"
  printf '%s\n' "${value:-6000}"
}

auth_host_port() {
  local value
  value="$(env_or_file_value AUTH_HOST_PORT)"
  printf '%s\n' "${value:-6001}"
}

nginx_api_upstream() {
  local value
  value="$(env_or_file_value NGINX_API_UPSTREAM)"
  printf '%s\n' "${value:-http://127.0.0.1:$(api_host_port)}"
}

nginx_auth_upstream() {
  local value
  value="$(env_or_file_value NGINX_AUTH_UPSTREAM)"
  printf '%s\n' "${value:-http://127.0.0.1:$(auth_host_port)}"
}

nginx_auth_health_upstream() {
  local value
  local auth_upstream
  value="$(env_or_file_value NGINX_AUTH_HEALTH_UPSTREAM)"
  if [[ -n "$value" ]]; then
    printf '%s\n' "$value"
    return
  fi
  auth_upstream="$(nginx_auth_upstream)"
  printf '%s/api/health\n' "${auth_upstream%/}"
}

nginx_real_ip_include() {
  env_or_file_value NGINX_REAL_IP_INCLUDE
}

image_source_repo_from_ref() {
  local value="$1"
  local image_path
  local owner
  local rest
  local repo

  image_path="${value%@sha256:*}"
  image_path="${image_path#ghcr.io/}"
  owner="${image_path%%/*}"
  rest="${image_path#*/}"
  repo="${rest%%/*}"
  [[ -n "$owner" && -n "$repo" && "$rest" != "$image_path" ]] || return 1
  printf '%s/%s\n' "$owner" "$repo"
}

image_owner_from_ref() {
  local value="$1"
  local repo
  repo="$(image_source_repo_from_ref "$value")" || return 1
  printf '%s\n' "${repo%%/*}"
}

ensure_env_file() {
  if [[ ! -f "$ENV_FILE" ]]; then
    [[ -f "$ENV_EXAMPLE" ]] || die "missing $ENV_EXAMPLE"
    cp "$ENV_EXAMPLE" "$ENV_FILE"
    echo "Created $ENV_FILE"
  fi

  for key in POSTGRES_PASSWORD AUTH_INTERNAL_KEY BETTER_AUTH_SECRET; do
    if is_placeholder "$(env_value "$key")"; then
      set_env_value "$key" "$(generate_secret)"
      echo "Generated $key"
    fi
  done
  if is_placeholder "$(env_value NATS_PASSWORD)"; then
    set_env_value NATS_PASSWORD "$(generate_url_safe_secret)"
    echo "Generated NATS_PASSWORD"
  fi
}

validate_env_file() {
  local missing=()
  local key

  for key in \
    APP_BASE_URL \
    API_BASE_URL \
    IDEABOARD_API_IMAGE \
    IDEABOARD_AUTH_IMAGE \
    IDEABOARD_AUTH_MIGRATE_IMAGE \
    POSTGRES_PASSWORD \
    AUTH_INTERNAL_KEY \
    BETTER_AUTH_SECRET \
    NATS_PASSWORD; do
    if is_placeholder "$(env_value "$key")"; then
      missing+=("$key")
    fi
  done

  if (( ${#missing[@]} > 0 )); then
    echo "Please edit $ENV_FILE and replace these values:" >&2
    printf '  - %s\n' "${missing[@]}" >&2
    return 1
  fi

  local api_image
  local auth_image
  local auth_migrate_image
  local api_repo
  local auth_repo
  local auth_migrate_repo
  local nats_user
  local nats_password
  api_image="$(env_value IDEABOARD_API_IMAGE)"
  auth_image="$(env_value IDEABOARD_AUTH_IMAGE)"
  auth_migrate_image="$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)"

  if ! is_ideaboard_image_ref "$api_image" "api"; then
    echo "Please set IDEABOARD_API_IMAGE to a full immutable GHCR digest ref, e.g. ghcr.io/owner/repo/ideaboard-api@sha256:..." >&2
    return 1
  fi
  if ! is_ideaboard_image_ref "$auth_image" "auth"; then
    echo "Please set IDEABOARD_AUTH_IMAGE to a full immutable GHCR digest ref, e.g. ghcr.io/owner/repo/ideaboard-auth@sha256:..." >&2
    return 1
  fi
  if ! is_ideaboard_image_ref "$auth_migrate_image" "auth-migrate"; then
    echo "Please set IDEABOARD_AUTH_MIGRATE_IMAGE to a full immutable GHCR digest ref, e.g. ghcr.io/owner/repo/ideaboard-auth-migrate@sha256:..." >&2
    return 1
  fi
  api_repo="$(image_source_repo_from_ref "$api_image")"
  auth_repo="$(image_source_repo_from_ref "$auth_image")"
  auth_migrate_repo="$(image_source_repo_from_ref "$auth_migrate_image")"
  if ! [[ "$api_repo" == "$auth_repo" && "$api_repo" == "$auth_migrate_repo" ]]; then
    echo "IDEABOARD_API_IMAGE, IDEABOARD_AUTH_IMAGE, and IDEABOARD_AUTH_MIGRATE_IMAGE must come from the same GHCR owner/repo." >&2
    return 1
  fi
  nats_user="$(env_value NATS_USER)"
  nats_user="${nats_user:-ideaboard}"
  nats_password="$(env_value NATS_PASSWORD)"
  if ! is_url_userinfo_value "$nats_user"; then
    echo "NATS_USER may only contain URL-safe characters: A-Z a-z 0-9 . _ ~ -" >&2
    return 1
  fi
  if ! is_url_userinfo_value "$nats_password"; then
    echo "NATS_PASSWORD may only contain URL-safe characters: A-Z a-z 0-9 . _ ~ -" >&2
    return 1
  fi
  return 0
}

docker_compose() {
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
}

wait_for_postgres() {
  local timeout="${POSTGRES_WAIT_TIMEOUT_SECONDS:-120}"
  local elapsed=0

  echo "Waiting for PostgreSQL to become ready..."
  while (( elapsed < timeout )); do
    # shellcheck disable=SC2016 # Container env vars must expand inside postgres.
    if docker_compose exec -T postgres sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" -h 127.0.0.1' >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done

  docker_compose logs --tail=120 postgres >&2 || true
  die "PostgreSQL did not become ready within ${timeout}s"
}

prune_backups() {
  local pattern="$1"
  local retain="$BACKUP_RETAIN"
  local backups=()
  local backup
  local delete_count
  local i

  [[ "$retain" =~ ^[0-9]+$ ]] || retain=5
  (( retain > 0 )) || retain=5

  while IFS= read -r backup; do
    backups+=("$backup")
  done < <(find "$BACKUP_DIR" -type f -name "$pattern" | sort)

  delete_count=$(( ${#backups[@]} - retain ))
  if (( delete_count <= 0 )); then
    return
  fi

  for (( i = 0; i < delete_count; i++ )); do
    rm -f -- "${backups[$i]}"
  done
}

finalize_backup_file() {
  local tmp_file="$1"
  local backup_file="$2"
  local prune_pattern="$3"
  local success_message="$4"
  local failure_message="$5"

  if ! mv "$tmp_file" "$backup_file"; then
    rm -f -- "$tmp_file"
    die "$failure_message"
  fi
  chmod 600 "$backup_file"
  prune_backups "$prune_pattern"
  echo "$success_message"
}

backup_postgres() {
  local timestamp
  local database
  local backup_file
  local tmp_file

  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  timestamp="$(date +%Y%m%d-%H%M%S)"
  database="$(env_value POSTGRES_DB)"
  database="${database:-ideaboard}"
  backup_file="$BACKUP_DIR/postgres-${database}-${timestamp}.sql"
  tmp_file="${backup_file}.tmp"

  echo "Creating PostgreSQL backup: $backup_file"
  # shellcheck disable=SC2016 # Container env vars must expand inside postgres.
  if docker_compose exec -T postgres sh -c 'pg_dump --clean --if-exists --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$tmp_file"; then
    finalize_backup_file \
      "$tmp_file" \
      "$backup_file" \
      'postgres-*.sql' \
      "PostgreSQL backup completed: $backup_file" \
      "PostgreSQL backup finalize failed; deployment aborted before migration"
    return 0
  fi

  rm -f -- "$tmp_file"
  die "PostgreSQL backup failed; deployment aborted before migration"
}

backup_api_uploads() {
  local timestamp
  local backup_file
  local tmp_file
  local volume_name

  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  timestamp="$(date +%Y%m%d-%H%M%S)"
  backup_file="$BACKUP_DIR/api-uploads-${timestamp}.tar.gz"
  tmp_file="${backup_file}.tmp"
  volume_name="${COMPOSE_PROJECT_NAME}_api-uploads"

  echo "Creating api-uploads backup: $backup_file"
  if docker volume inspect "$volume_name" >/dev/null 2>&1; then
    if docker run --rm \
      -v "$volume_name:/data:ro" \
      -v "$BACKUP_DIR:/backup" \
      postgres:17-alpine \
      sh -c "tar -C /data -czf /backup/$(basename "$tmp_file") ." ; then
      finalize_backup_file \
        "$tmp_file" \
        "$backup_file" \
        'api-uploads-*.tar.gz' \
        "api-uploads backup completed: $backup_file" \
        "api-uploads backup finalize failed; deployment aborted before migration"
      return 0
    fi
    rm -f -- "$tmp_file"
    die "api-uploads backup failed; deployment aborted before migration"
  fi

  if tar -czf "$tmp_file" --files-from /dev/null; then
    finalize_backup_file \
      "$tmp_file" \
      "$backup_file" \
      'api-uploads-*.tar.gz' \
      "api-uploads volume not found; wrote empty archive: $backup_file" \
      "api-uploads empty backup finalize failed; deployment aborted before migration"
    return 0
  fi

  rm -f -- "$tmp_file"
  die "api-uploads empty backup failed; deployment aborted before migration"
}

print_backend_health_logs() {
  echo "Recent service logs for health gate failure:" >&2
  docker_compose ps >&2 || true
  docker_compose logs --tail=120 api auth postgres nats >&2 || true
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

wait_for_http_local() {
  local name="$1"
  local url="$2"
  local timeout="${HEALTH_WAIT_TIMEOUT_SECONDS:-120}"
  local elapsed=0

  echo "Waiting for $name health: $url"
  while (( elapsed < timeout )); do
    if http_get_quiet "$url"; then
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
  local api_upstream
  api_upstream="$(nginx_api_upstream)"
  wait_for_http_local "api upstream" "${api_upstream%/}/api/health"
  wait_for_http_local "auth upstream" "$(nginx_auth_health_upstream)"
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
    # shellcheck disable=SC2016 # Container env vars must expand inside postgres.
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

http_get_quiet() {
  local url="$1"

  if command -v curl >/dev/null 2>&1; then
    curl -fs --max-time 5 "$url" >/dev/null
    return
  fi

  command -v wget >/dev/null 2>&1 || die "curl or wget is required for public API health checks"
  wget -q -T 5 -O /dev/null "$url"
}

public_api_health_url() {
  local base_url="$1"

  if [[ "$base_url" == */api ]]; then
    printf '%s/health\n' "$base_url"
    return
  fi
  printf '%s/api/health\n' "$base_url"
}

check_public_api_health() {
  local base_url
  local public_health_url
  local timeout="${PUBLIC_API_HEALTH_WAIT_TIMEOUT_SECONDS:-120}"
  local elapsed=0

  base_url="$(env_value API_BASE_URL)"
  base_url="${base_url%/}"
  [[ -n "$base_url" ]] || die "API_BASE_URL is required for public API health checks"

  public_health_url="$(public_api_health_url "$base_url")"
  echo "Waiting for public API health: $public_health_url"
  while (( elapsed < timeout )); do
    if http_get_quiet "$public_health_url"; then
      echo "public API health is OK"
      return 0
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done

  print_backend_health_logs
  die "public API health gate failed after ${timeout}s: $public_health_url"
}

run_backend_smoke_checks() {
  wait_for_postgres
  wait_for_nats
  wait_for_backend_health
  check_river_schema
  check_nats_event_stream
  check_public_api_health
  echo "Backend smoke checks passed"
}

login_ghcr_if_configured() {
  if [[ -z "${GHCR_TOKEN:-}" ]]; then
    return
  fi

  local username
  username="${GHCR_USERNAME:-$(image_owner_from_ref "$(env_value IDEABOARD_API_IMAGE)")}"
  [[ -n "$username" ]] || die "GHCR_TOKEN is set, but GHCR_USERNAME or IDEABOARD_API_IMAGE is empty"

  printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u "$username" --password-stdin
}

cosign_verify_enabled() {
  local value
  value="$(env_value COSIGN_VERIFY)"
  if [[ -z "$value" ]]; then
    value="true"
  fi
  ! is_false_value "$value"
}

verify_image_signature() {
  local image_ref="$1"
  local identity_regexp
  local oidc_issuer
  local source_repo
  local source_repo_regexp

  if ! cosign_verify_enabled; then
    echo "Skipping cosign verification for $image_ref because COSIGN_VERIFY=false"
    return 0
  fi
  command -v cosign >/dev/null 2>&1 || die "cosign is required to verify image signatures. Install cosign or set COSIGN_VERIFY=false only for temporary offline recovery."

  identity_regexp="$(env_value COSIGN_CERTIFICATE_IDENTITY_REGEXP)"
  if [[ -z "$identity_regexp" ]]; then
    source_repo="$(image_source_repo_from_ref "$image_ref")" || die "cannot derive GitHub source repository from image ref: $image_ref"
    source_repo_regexp="${source_repo//./\\.}"
    identity_regexp="https://github.com/${source_repo_regexp}/\\.github/workflows/docker-images\\.yml@refs/(heads|tags)/.+"
  fi
  oidc_issuer="$(env_value COSIGN_CERTIFICATE_OIDC_ISSUER)"
  oidc_issuer="${oidc_issuer:-https://token.actions.githubusercontent.com}"

  echo "Verifying image signature: $image_ref"
  cosign verify \
    --certificate-identity-regexp "$identity_regexp" \
    --certificate-oidc-issuer "$oidc_issuer" \
    "$image_ref" >/dev/null
}

verify_image_signatures() {
  verify_image_signature "$(env_value IDEABOARD_API_IMAGE)"
  verify_image_signature "$(env_value IDEABOARD_AUTH_IMAGE)"
  verify_image_signature "$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)"
}

prepare_stack_images() {
  login_ghcr_if_configured
  verify_image_signatures
  docker_compose --profile tools pull
}

run_auth_migration() {
  docker_compose run --rm auth-migrate
}

run_api_migration() {
  docker_compose run --rm api-migrate
}

render_host_nginx_config() {
  local output_path="${1:-$ROOT_DIR/deploy/nginx/ideaboard.conf}"

  NGINX_API_UPSTREAM="$(nginx_api_upstream)" \
    NGINX_AUTH_UPSTREAM="$(nginx_auth_upstream)" \
    NGINX_AUTH_HEALTH_UPSTREAM="$(nginx_auth_health_upstream)" \
    NGINX_REAL_IP_INCLUDE="$(nginx_real_ip_include)" \
    NGINX_WEB_ROOT="${NGINX_WEB_ROOT:-/usr/share/nginx/html}" \
    NGINX_ROOT_DIRECTIVE="${NGINX_ROOT_DIRECTIVE:-return 404}" \
    bash "$ROOT_DIR/scripts/render-nginx-template.sh" "$output_path"
  echo "Rendered Nginx config: $output_path"
}

render_cloudflare_real_ip_config() {
  local output_path="${1:-$ROOT_DIR/deploy/nginx/cloudflare-real-ip.conf}"

  bash "$ROOT_DIR/scripts/render-cloudflare-real-ip-conf.sh" "$output_path"
}

detect_registry_repo() {
  # 1. Try to read from git remote if in a git repo
  if [[ -d "$ROOT_DIR/.git" ]] && command -v git >/dev/null 2>&1; then
    local remote_url
    remote_url=$(git -C "$ROOT_DIR" remote get-url origin 2>/dev/null || true)
    if [[ -n "$remote_url" ]]; then
      local clean_url="${remote_url%.git}"
      local owner_repo=""
      if [[ "$clean_url" =~ github\.com[:/]([^/]+/[^/]+) ]]; then
        owner_repo="${BASH_REMATCH[1]}"
      fi
      if [[ -n "$owner_repo" ]]; then
        owner_repo=$(echo "$owner_repo" | tr '[:upper:]' '[:lower:]')
        echo "ghcr.io/$owner_repo"
        return 0
      fi
    fi
  fi

  # 2. Try to parse from existing IDEABOARD_API_IMAGE in .env if not placeholder
  local api_image
  api_image=$(env_value IDEABOARD_API_IMAGE)
  if [[ -n "$api_image" ]] && ! is_placeholder "$api_image"; then
    local repo
    repo=$(image_source_repo_from_ref "$api_image")
    if [[ -n "$repo" ]]; then
      echo "ghcr.io/$repo"
      return 0
    fi
  fi

  return 1
}

detect_update_tag() {
  local tag
  tag=$(env_value IDEABOARD_UPDATE_TAG)
  if [[ -n "$tag" ]] && ! is_placeholder "$tag"; then
    echo "$tag"
    return 0
  fi

  if [[ -d "$ROOT_DIR/.git" ]] && command -v git >/dev/null 2>&1; then
    local branch
    branch=$(git -C "$ROOT_DIR" symbolic-ref --short -q HEAD 2>/dev/null || true)
    if [[ -n "$branch" ]]; then
      if [[ "$branch" == "main" || "$branch" == "master" ]]; then
        echo "latest"
      else
        echo "$branch"
      fi
      return 0
    fi

    local tag_name
    tag_name=$(git -C "$ROOT_DIR" describe --tags --exact-match 2>/dev/null || true)
    if [[ -n "$tag_name" ]]; then
      echo "$tag_name"
      return 0
    fi
  fi

  echo "latest"
}

resolve_digest_for_image() {
  local component="$1"
  local registry_repo="$2"
  local tag="$3"
  local image_tag_ref="${registry_repo}/ideaboard-${component}:${tag}"

  echo "Pulling image for ${component} (tag: ${tag})..." >&2
  if ! docker pull "$image_tag_ref" >&2; then
    echo "Warning: failed to pull ${image_tag_ref}" >&2
    return 1
  fi

  local digest_ref
  digest_ref=$(docker inspect --format='{{range .RepoDigests}}{{.}} {{end}}' "$image_tag_ref" | tr ' ' '\n' | grep -F "${registry_repo}/ideaboard-${component}@sha256:" | head -n 1)
  if [[ -z "$digest_ref" ]]; then
    echo "Warning: failed to find digest for ${image_tag_ref}" >&2
    return 1
  fi

  echo "$digest_ref"
}

perform_auto_update() {
  echo "=== Running Auto-Update ==="

  # 1. Update local git repository if on a branch
  if [[ -d "$ROOT_DIR/.git" ]] && command -v git >/dev/null 2>&1; then
    local branch
    branch=$(git -C "$ROOT_DIR" symbolic-ref --short -q HEAD 2>/dev/null || true)
    if [[ -n "$branch" ]]; then
      echo "Updating local codebase from git remote (branch: $branch)..."
      if ! git -C "$ROOT_DIR" pull --ff-only; then
        echo "Warning: git pull failed. Proceeding with current local files."
      fi
    fi
  fi

  # 2. Detect registry and tag
  local registry_repo
  registry_repo=$(detect_registry_repo) || die "Could not detect GHCR registry repository prefix. Please ensure git remote is set or IDEABOARD_API_IMAGE is configured in $ENV_FILE"

  local tag
  tag=$(detect_update_tag)

  echo "Update channel/tag: $tag"
  echo "Registry repository: $registry_repo"

  # 3. Login to GHCR if configured
  login_ghcr_if_configured

  # 4. Resolve digests
  local api_digest auth_digest auth_migrate_digest
  api_digest=$(resolve_digest_for_image "api" "$registry_repo" "$tag") || die "Failed to resolve digest for api image"
  auth_digest=$(resolve_digest_for_image "auth" "$registry_repo" "$tag") || die "Failed to resolve digest for auth image"
  auth_migrate_digest=$(resolve_digest_for_image "auth-migrate" "$registry_repo" "$tag") || die "Failed to resolve digest for auth-migrate image"

  # 5. Write back to .env
  set_env_value IDEABOARD_API_IMAGE "$api_digest"
  set_env_value IDEABOARD_AUTH_IMAGE "$auth_digest"
  set_env_value IDEABOARD_AUTH_MIGRATE_IMAGE "$auth_migrate_digest"

  echo "Successfully resolved and updated image digests in $ENV_FILE"
  echo "==========================="
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

if [[ "$command" == "nginx-conf" ]]; then
  shift || true
  render_host_nginx_config "${1:-}"
  exit 0
fi

if [[ "$command" == "cloudflare-real-ip-conf" ]]; then
  shift || true
  render_cloudflare_real_ip_config "${1:-}"
  exit 0
fi

if [[ "$is_update" == "1" ]] || \
   is_placeholder "$(env_value IDEABOARD_API_IMAGE)" || \
   is_placeholder "$(env_value IDEABOARD_AUTH_IMAGE)" || \
   is_placeholder "$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)" || \
   ! is_ideaboard_image_ref "$(env_value IDEABOARD_API_IMAGE)" "api" || \
   ! is_ideaboard_image_ref "$(env_value IDEABOARD_AUTH_IMAGE)" "auth" || \
   ! is_ideaboard_image_ref "$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)" "auth-migrate"; then
  perform_auto_update
fi

validate_env_file || exit 1

case "$command" in
  deploy)
    prepare_stack_images
    docker_compose up -d postgres
    wait_for_postgres
    backup_postgres
    backup_api_uploads
    run_api_migration
    run_auth_migration
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
    prepare_stack_images
    ;;
  up)
    docker_compose up -d
    ;;
  migrate)
    prepare_stack_images
    docker_compose up -d postgres
    wait_for_postgres
    backup_postgres
    backup_api_uploads
    run_api_migration
    run_auth_migration
    ;;
  ps)
    docker_compose ps
    ;;
  *)
    cat >&2 <<EOF
Usage: ideaboard [update|smoke|restart|stop|status|logs|delete|uninstall|init-env|pull|up|migrate|ps]
       ideaboard nginx-conf [output-path]
       ideaboard cloudflare-real-ip-conf [output-path]

Default command: update
Environment file: $ENV_FILE
EOF
    exit 1
    ;;
esac
