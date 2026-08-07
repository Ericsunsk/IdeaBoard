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
RELEASE_STATE_DIR="${RELEASE_STATE_DIR:-$ROOT_DIR/docker/releases}"
RELEASE_HISTORY_DIR="$RELEASE_STATE_DIR/history"
CURRENT_RELEASE_FILE="$RELEASE_STATE_DIR/current.json"
PREVIOUS_RELEASE_FILE="$RELEASE_STATE_DIR/previous.json"

requested_command="${1:-update}"
command="$requested_command"
is_update=0
if [[ "$command" == "update" ]]; then
  command="deploy"
  is_update=1
fi
update_manifest_source=""

if [[ "$requested_command" == "update" || "$requested_command" == "deploy" ]]; then
  args=("${@:2}")
  remaining_args=()
  i=0
  while (( i < ${#args[@]} )); do
    case "${args[$i]}" in
      --manifest)
        if (( i + 1 >= ${#args[@]} )); then
          echo "Error: --manifest requires a path or URL" >&2
          exit 1
        fi
        update_manifest_source="${args[$((i + 1))]}"
        i=$((i + 2))
        ;;
      --manifest=*)
        update_manifest_source="${args[$i]#--manifest=}"
        i=$((i + 1))
        ;;
      *)
        remaining_args+=("${args[$i]}")
        i=$((i + 1))
        ;;
    esac
  done
  if (( ${#remaining_args[@]} > 0 )); then
    echo "Error: unexpected arguments for $requested_command: ${remaining_args[*]}" >&2
    exit 1
  fi
fi

DEPLOY_STAGE="init"
DEPLOY_RELEASE_ID=""
DEPLOY_STARTED_AT=""
DEPLOY_GIT_REF=""
DEPLOY_GIT_COMMIT=""
DEPLOY_POSTGRES_BACKUP=""
DEPLOY_UPLOADS_BACKUP=""
DEPLOY_ENV_SNAPSHOT=""
DEPLOY_IMAGE_UPDATE_PENDING=0
DEPLOY_ENV_RESTORE_ALLOWED=1
DEPLOY_MIGRATION_STARTED=0
DEPLOY_TRAP_ENABLED=0
DEPLOY_SUCCEEDED=0
DEPLOY_PREFLIGHT_GATE="pending"
DEPLOY_COSIGN_GATE="pending"
DEPLOY_BACKUP_GATE="pending"
DEPLOY_MIGRATION_GATE="pending"
DEPLOY_SMOKE_GATE="pending"
DEPLOY_SOURCE_MANIFEST=""
DEPLOY_SOURCE_MANIFEST_COPY=""
DEPLOY_API_IMAGE=""
DEPLOY_AUTH_IMAGE=""
DEPLOY_AUTH_MIGRATE_IMAGE=""
PREVIOUS_API_IMAGE=""
PREVIOUS_AUTH_IMAGE=""
PREVIOUS_AUTH_MIGRATE_IMAGE=""

if [[ "$command" == "delete" || "$command" == "uninstall" ]]; then
  shift || true
  IDEABOARD_STACK_DIR="$ROOT_DIR" exec "$ROOT_DIR/scripts/uninstall-vps.sh" "$@"
fi

die() {
  echo "Error: $*" >&2
  exit 1
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

json_value() {
  printf '"%s"' "$(json_escape "$1")"
}

json_string_field() {
  local key="$1"
  local file="$2"
  awk -v key="$key" '
    $0 ~ "\"" key "\"[[:space:]]*:" {
      line = $0
      sub(".*\"" key "\"[[:space:]]*:[[:space:]]*\"", "", line)
      sub("\".*", "", line)
      print line
      exit
    }
  ' "$file"
}

json_object_string_field() {
  local object="$1"
  local key="$2"
  local file="$3"
  awk -v object="$object" -v key="$key" '
    $0 ~ "\"" object "\"[[:space:]]*:" { in_object = 1; next }
    in_object && /^[[:space:]]*}/ { exit }
    in_object && $0 ~ "\"" key "\"[[:space:]]*:" {
      line = $0
      sub(".*\"" key "\"[[:space:]]*:[[:space:]]*\"", "", line)
      sub("\".*", "", line)
      print line
      exit
    }
  ' "$file"
}

json_image_field() {
  local key="$1"
  local file="$2"
  json_object_string_field "images" "$key" "$file"
}

utc_now() {
  date -u '+%Y-%m-%dT%H:%M:%SZ'
}

release_timestamp() {
  date -u '+%Y%m%d-%H%M%S'
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
  tmp="$(mktemp "$(dirname "$ENV_FILE")/.env.XXXXXX")"
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
  chmod 600 "$ENV_FILE"
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

api_metrics_host_port() {
  local value
  value="$(env_or_file_value API_METRICS_HOST_PORT)"
  printf '%s\n' "${value:-9091}"
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
    chmod 600 "$ENV_FILE"
    echo "Created $ENV_FILE"
  fi

  chmod 600 "$ENV_FILE"

  for key in POSTGRES_PASSWORD API_DB_PASSWORD AUTH_DB_PASSWORD AUTH_INTERNAL_KEY APP_SECRETS_ENCRYPTION_KEY BETTER_AUTH_SECRET; do
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
    API_DB_PASSWORD \
    AUTH_DB_PASSWORD \
    AUTH_INTERNAL_KEY \
    APP_SECRETS_ENCRYPTION_KEY \
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
  nats_password="$(env_value NATS_PASSWORD)"
  if ! is_url_userinfo_value "$nats_password"; then
    echo "NATS_PASSWORD may only contain URL-safe characters: A-Z a-z 0-9 . _ ~ -" >&2
    return 1
  fi
  return 0
}

docker_compose() {
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
}

need_runtime_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

check_backup_dir_writable() {
  local probe
  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  probe="$(mktemp "$BACKUP_DIR/.write-test.XXXXXX")" || die "backup dir is not writable: $BACKUP_DIR"
  rm -f -- "$probe"
}

check_disk_space() {
  local min_mb="${DEPLOY_MIN_FREE_MB:-1024}"
  local available_kb
  [[ "$min_mb" =~ ^[0-9]+$ ]] || min_mb=1024
  available_kb="$(df -Pk "$BACKUP_DIR" | awk 'NR == 2 { print $4 }')"
  if [[ -n "$available_kb" && "$available_kb" =~ ^[0-9]+$ ]]; then
    if (( available_kb < min_mb * 1024 )); then
      die "backup filesystem has less than ${min_mb}MB free: $BACKUP_DIR"
    fi
  fi
}

check_backup_encryption_tool() {
  local recipient
  recipient="$(backup_encryption_recipient)"
  [[ -n "$recipient" ]] || return 0

  if [[ "$recipient" != gpg:* ]] && is_age_recipient "$recipient"; then
    command -v age >/dev/null 2>&1 || die "BACKUP_ENCRYPTION_RECIPIENT is configured for age, but age is not installed"
    return 0
  fi

  command -v gpg >/dev/null 2>&1 || die "BACKUP_ENCRYPTION_RECIPIENT is configured for gpg, but gpg is not installed"
}

check_port_available_or_used_by_compose() {
  local port="$1"
  local service_name="$2"
  local container_name="$3"
  local ports
  [[ "$port" =~ ^[0-9]+$ ]] || die "$service_name host port must be numeric: $port"
  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      ports="$(docker ps --filter "name=^/${container_name}$" --format '{{.Ports}}' 2>/dev/null || true)"
      if [[ "$ports" == *"127.0.0.1:${port}->"* || "$ports" == *"0.0.0.0:${port}->"* || "$ports" == *":::${port}->"* ]]; then
        echo "Port $port is already listening; assuming it belongs to the existing $service_name container."
        return 0
      fi
      die "port $port is already in use outside the expected $service_name container"
    fi
  fi
}

check_nginx_template_render() {
  local tmp
  tmp="$(mktemp)"
  render_host_nginx_config "$tmp" >/dev/null
  if grep -F "\${" "$tmp" >/dev/null; then
    rm -f -- "$tmp"
    die "rendered Nginx config still contains unresolved template variables"
  fi
  rm -f -- "$tmp"
}

preflight_checks() {
  echo "Running deployment preflight checks..."
  need_runtime_command docker
  need_runtime_command tar
  if ! docker compose version >/dev/null 2>&1; then
    die "missing command: docker compose"
  fi
  docker info >/dev/null 2>&1 || die "Docker daemon is not available"
  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    die "curl or wget is required"
  fi
  validate_env_file || exit 1
  docker_compose config >/dev/null
  check_backup_dir_writable
  check_disk_space
  check_backup_encryption_tool
  check_port_available_or_used_by_compose "$(api_host_port)" "api" "ideaboard-api"
  check_port_available_or_used_by_compose "$(api_metrics_host_port)" "api metrics" "ideaboard-api"
  check_port_available_or_used_by_compose "$(auth_host_port)" "auth" "ideaboard-auth"
  if [[ -n "$(nginx_real_ip_include)" && ! -r "$(nginx_real_ip_include)" ]]; then
    die "NGINX_REAL_IP_INCLUDE is not readable: $(nginx_real_ip_include)"
  fi
  check_nginx_template_render
  DEPLOY_PREFLIGHT_GATE="passed"
  echo "Preflight checks passed"
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
  done < <(find "$BACKUP_DIR" -type f -name "$pattern" ! -name '*.sha256' ! -name '*.tmp' | sort)

  delete_count=$(( ${#backups[@]} - retain ))
  if (( delete_count <= 0 )); then
    return
  fi

  for (( i = 0; i < delete_count; i++ )); do
    rm -f -- "${backups[$i]}" "${backups[$i]}.sha256"
  done
}

checksum_file() {
  local file="$1"
  local checksum_file="${file}.sha256"
  local tmp_file="${checksum_file}.tmp"
  local digest

  if command -v sha256sum >/dev/null 2>&1; then
    digest="$(sha256sum "$file" | awk '{ print $1 }')"
  elif command -v shasum >/dev/null 2>&1; then
    digest="$(shasum -a 256 "$file" | awk '{ print $1 }')"
  else
    die "sha256sum or shasum is required to checksum backups"
  fi

  [[ -n "$digest" ]] || die "failed to calculate SHA-256 for backup: $file"
  printf '%s  %s\n' "$digest" "$(basename "$file")" > "$tmp_file"
  mv "$tmp_file" "$checksum_file"
  chmod 600 "$checksum_file"
}

backup_checksum_status() {
  local file="$1"
  local checksum_file="${file}.sha256"
  local checksum_name
  local backup_dir

  [[ -f "$checksum_file" ]] || {
    printf 'missing'
    return 0
  }

  checksum_name="$(basename "$checksum_file")"
  backup_dir="$(dirname "$file")"
  if command -v sha256sum >/dev/null 2>&1; then
    if (cd "$backup_dir" && sha256sum -c "$checksum_name" >/dev/null 2>&1); then
      printf 'valid'
    else
      printf 'invalid'
    fi
    return 0
  fi

  if command -v shasum >/dev/null 2>&1; then
    if (cd "$backup_dir" && shasum -a 256 -c "$checksum_name" >/dev/null 2>&1); then
      printf 'valid'
    else
      printf 'invalid'
    fi
    return 0
  fi

  printf 'unchecked'
}

backup_encryption_recipient() {
  env_or_file_value BACKUP_ENCRYPTION_RECIPIENT
}

is_age_recipient() {
  local recipient="$1"
  [[ "$recipient" == age1* || "$recipient" == ssh-* || "$recipient" == age:* ]]
}

encrypt_backup_file() {
  local backup_file="$1"
  local recipient
  local clean_recipient
  local encrypted_file
  recipient="$(backup_encryption_recipient)"
  [[ -n "$recipient" ]] || {
    printf '%s\n' "$backup_file"
    return 0
  }

  clean_recipient="$recipient"
  if [[ "$recipient" == age:* ]]; then
    clean_recipient="${recipient#age:}"
  elif [[ "$recipient" == gpg:* ]]; then
    clean_recipient="${recipient#gpg:}"
  fi
  [[ -n "$clean_recipient" ]] || die "BACKUP_ENCRYPTION_RECIPIENT is empty after prefix parsing"

  if [[ "$recipient" != gpg:* ]] && is_age_recipient "$recipient"; then
    command -v age >/dev/null 2>&1 || {
      rm -f -- "$backup_file"
      die "BACKUP_ENCRYPTION_RECIPIENT requires age, but age is not installed"
    }
    encrypted_file="${backup_file}.age"
    age -r "$clean_recipient" -o "$encrypted_file" "$backup_file" || {
      rm -f -- "$backup_file" "$encrypted_file"
      die "age encryption failed for backup: $backup_file"
    }
  else
    command -v gpg >/dev/null 2>&1 || {
      rm -f -- "$backup_file"
      die "BACKUP_ENCRYPTION_RECIPIENT requires gpg, but gpg is not installed"
    }
    encrypted_file="${backup_file}.gpg"
    gpg --batch --yes --trust-model always --recipient "$clean_recipient" --encrypt --output "$encrypted_file" "$backup_file" || {
      rm -f -- "$backup_file" "$encrypted_file"
      die "gpg encryption failed for backup: $backup_file"
    }
  fi

  chmod 600 "$encrypted_file"
  rm -f -- "$backup_file"
  printf '%s\n' "$encrypted_file"
}

finalize_backup_file() {
  local tmp_file="$1"
  local backup_file="$2"
  local prune_pattern="$3"
  local success_message="$4"
  local failure_message="$5"
  local final_file

  if ! mv "$tmp_file" "$backup_file"; then
    rm -f -- "$tmp_file"
    die "$failure_message"
  fi
  chmod 600 "$backup_file"
  final_file="$(encrypt_backup_file "$backup_file")"
  checksum_file "$final_file"
  prune_backups "$prune_pattern"
  echo "${success_message/$backup_file/$final_file}" >&2
  printf '%s\n' "$final_file"
}

verify_postgres_custom_backup() {
  local backup_file="$1"

  if ! docker_compose exec -T postgres sh -c 'command -v pg_restore >/dev/null 2>&1'; then
    echo "pg_restore is missing in the postgres container; cannot verify custom PostgreSQL backup" >&2
    return 1
  fi

  docker_compose exec -T postgres pg_restore --list < "$backup_file" >/dev/null || {
    echo "PostgreSQL custom backup verification failed: $backup_file" >&2
    return 1
  }
}

verify_tar_gz_backup() {
  local backup_file="$1"
  tar -tzf "$backup_file" >/dev/null || {
    echo "tar backup verification failed: $backup_file" >&2
    return 1
  }
}

compose_declared_volume_name() {
  local source_name="$1"

  [[ -n "$source_name" ]] || return 0
  docker_compose config 2>/dev/null | awk -v source="$source_name" '
    /^volumes:/ {
      in_volumes = 1
      next
    }
    in_volumes && /^[^[:space:]]/ {
      exit
    }
    in_volumes && $0 ~ "^  " source ":" {
      in_target = 1
      next
    }
    in_target && /^  [^[:space:]]/ {
      exit
    }
    in_target && /^[[:space:]]+name:/ {
      line = $0
      sub("^[[:space:]]+name:[[:space:]]*", "", line)
      gsub("\"", "", line)
      print line
      exit
    }
  ' || true
}

compose_service_uploads_source() {
  if command -v jq >/dev/null 2>&1; then
    docker_compose config --format json 2>/dev/null \
      | jq -r '.services.api.volumes[]? | select(.target == "/app/uploads") | .source // empty' \
      | head -n 1 \
      || true
    return 0
  fi

  docker_compose config 2>/dev/null | awk '
    /^services:/ {
      in_services = 1
      next
    }
    in_services && /^  api:/ {
      in_api = 1
      next
    }
    in_api && /^  [^[:space:]]/ {
      exit
    }
    in_api && /^    volumes:/ {
      in_volumes = 1
      next
    }
    in_volumes && /^    [^[:space:]]/ {
      in_volumes = 0
    }
    in_volumes && /^[[:space:]]+-[[:space:]]+type:[[:space:]]+volume/ {
      in_mount = 1
      source = ""
      target = ""
      next
    }
    in_mount && /^[[:space:]]+-[[:space:]]+/ {
      in_mount = 0
    }
    in_mount && /^[[:space:]]+source:/ {
      line = $0
      sub("^[[:space:]]+source:[[:space:]]*", "", line)
      gsub("\"", "", line)
      source = line
    }
    in_mount && /^[[:space:]]+target:/ {
      line = $0
      sub("^[[:space:]]+target:[[:space:]]*", "", line)
      gsub("\"", "", line)
      target = line
    }
    in_mount && source != "" && target == "/app/uploads" {
      print source
      exit
    }
  ' || true
}

backup_postgres() {
  local timestamp
  local database
  local backup_file
  local tmp_file

  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  timestamp="$(date +%Y%m%d-%H%M%S)"
  database="ideaboard"
  backup_file="$BACKUP_DIR/postgres-${database}-${timestamp}.dump"
  tmp_file="${backup_file}.tmp"

  echo "Creating PostgreSQL backup: $backup_file"
  # shellcheck disable=SC2016 # Container env vars must expand inside postgres.
  if docker_compose exec -T postgres sh -c 'pg_dump --format=custom --clean --if-exists --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$tmp_file"; then
    if ! verify_postgres_custom_backup "$tmp_file"; then
      rm -f -- "$tmp_file"
      die "PostgreSQL backup verification failed; deployment aborted before migration"
    fi
    DEPLOY_POSTGRES_BACKUP="$(
      finalize_backup_file \
      "$tmp_file" \
      "$backup_file" \
      'postgres-*.dump*' \
      "PostgreSQL backup completed: $backup_file" \
      "PostgreSQL backup finalize failed; deployment aborted before migration"
    )"
    return 0
  fi

  rm -f -- "$tmp_file"
  die "PostgreSQL backup failed; deployment aborted before migration"
}

resolve_compose_volume_name() {
  local source_name="$1"
  local volume_name
  local declared_volume_name

  [[ -n "$source_name" ]] || return 0
  if docker volume inspect "$source_name" >/dev/null 2>&1; then
    printf '%s\n' "$source_name"
    return 0
  fi

  declared_volume_name="$(compose_declared_volume_name "$source_name")"
  if [[ -n "$declared_volume_name" ]] && docker volume inspect "$declared_volume_name" >/dev/null 2>&1; then
    printf '%s\n' "$declared_volume_name"
    return 0
  fi

  volume_name="$(docker volume ls -q --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" --filter "label=com.docker.compose.volume=$source_name" 2>/dev/null | head -n 1 || true)"
  if [[ -n "$volume_name" ]]; then
    printf '%s\n' "$volume_name"
    return 0
  fi

  volume_name="${COMPOSE_PROJECT_NAME}_${source_name}"
  if docker volume inspect "$volume_name" >/dev/null 2>&1; then
    printf '%s\n' "$volume_name"
    return 0
  fi

  return 0
}

api_container_uploads_volume_name() {
  local container_id
  local volume_name

  while IFS= read -r container_id; do
    [[ -n "$container_id" ]] || continue
    volume_name="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/app/uploads"}}{{.Name}}{{end}}{{end}}' "$container_id" 2>/dev/null || true)"
    if [[ -n "$volume_name" ]]; then
      printf '%s\n' "$volume_name"
      return 0
    fi
  done < <(
    {
      docker ps -a \
        --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" \
        --filter "label=com.docker.compose.service=api" \
        --format '{{.ID}}' 2>/dev/null || true
      docker ps -a --filter "name=^/ideaboard-api$" --format '{{.ID}}' 2>/dev/null || true
    } | awk 'NF && !seen[$0]++'
  )

  return 0
}

compose_uploads_volume_name() {
  local source_name=""
  local volume_name=""

  source_name="$(compose_service_uploads_source)"
  volume_name="$(api_container_uploads_volume_name)"

  if [[ -z "$volume_name" && -n "$source_name" ]]; then
    volume_name="$(resolve_compose_volume_name "$source_name")"
  fi

  if [[ -z "$volume_name" ]]; then
    volume_name="$(docker volume ls -q --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" --filter "label=com.docker.compose.volume=api-uploads" 2>/dev/null | head -n 1 || true)"
  fi

  printf '%s\n' "$volume_name"
}

api_uploads_state_exists() {
  local fallback_volume="${COMPOSE_PROJECT_NAME}_api-uploads"
  local declared_volume_name

  docker_compose ps -a -q api 2>/dev/null | grep -q . && return 0
  docker ps -a --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" --filter "label=com.docker.compose.service=api" --format '{{.ID}}' 2>/dev/null | grep -q . && return 0
  docker ps -a --filter "name=^/ideaboard-api$" --format '{{.ID}}' 2>/dev/null | grep -q . && return 0
  docker volume ls -q --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME" --filter "label=com.docker.compose.volume=api-uploads" 2>/dev/null | grep -q . && return 0
  declared_volume_name="$(compose_declared_volume_name "api-uploads")"
  [[ -n "$declared_volume_name" ]] && docker volume inspect "$declared_volume_name" >/dev/null 2>&1 && return 0
  docker volume inspect "$fallback_volume" >/dev/null 2>&1 && return 0
  return 1
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
  volume_name="$(compose_uploads_volume_name)"

  if [[ -n "$volume_name" ]]; then
    echo "Creating api-uploads backup from volume ${volume_name}: $backup_file"
  else
    echo "Creating api-uploads backup: no existing volume resolved"
  fi
  if [[ -n "$volume_name" ]] && docker volume inspect "$volume_name" >/dev/null 2>&1; then
    if docker run --rm \
      -v "$volume_name:/data:ro" \
      -v "$BACKUP_DIR:/backup" \
      postgres:17-alpine \
      sh -c "tar -C /data -czf /backup/$(basename "$tmp_file") ." ; then
      if ! verify_tar_gz_backup "$tmp_file"; then
        rm -f -- "$tmp_file"
        die "api-uploads backup verification failed; deployment aborted before migration"
      fi
      DEPLOY_UPLOADS_BACKUP="$(
        finalize_backup_file \
        "$tmp_file" \
        "$backup_file" \
        'api-uploads-*.tar.gz*' \
        "api-uploads backup completed: $backup_file" \
        "api-uploads backup finalize failed; deployment aborted before migration"
      )"
      return 0
    fi
    rm -f -- "$tmp_file"
    die "api-uploads backup failed; deployment aborted before migration"
  fi

  if api_uploads_state_exists; then
    die "api-uploads volume could not be resolved; refusing to write an empty backup"
  fi
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable; refusing to write an empty api-uploads backup"

  if tar -czf "$tmp_file" --files-from /dev/null; then
    if ! verify_tar_gz_backup "$tmp_file"; then
      rm -f -- "$tmp_file"
      die "api-uploads empty backup verification failed; deployment aborted before migration"
    fi
    DEPLOY_UPLOADS_BACKUP="$(
      finalize_backup_file \
      "$tmp_file" \
      "$backup_file" \
      'api-uploads-*.tar.gz*' \
      "api-uploads volume not found; wrote empty archive: $backup_file" \
      "api-uploads empty backup finalize failed; deployment aborted before migration"
    )"
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
  wait_for_http_local "api metrics" "http://127.0.0.1:$(api_metrics_host_port)/metrics"
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

  stream_name="IDEABOARD_EVENTS"

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

http_reaches_server() {
  local url="$1"
  local status
  local output

  if command -v curl >/dev/null 2>&1; then
    status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$url" || true)"
    [[ "$status" =~ ^[1-5][0-9][0-9]$ ]]
    return
  fi

  command -v wget >/dev/null 2>&1 || die "curl or wget is required for public HTTP checks"
  output="$(wget -S -T 5 -O /dev/null "$url" 2>&1 || true)"
  grep -E 'HTTP/[0-9.]+[[:space:]]+[1-5][0-9][0-9]' <<< "$output" >/dev/null
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

current_git_ref() {
  if [[ -d "$ROOT_DIR/.git" ]] && command -v git >/dev/null 2>&1; then
    git -C "$ROOT_DIR" symbolic-ref --short -q HEAD 2>/dev/null || \
      git -C "$ROOT_DIR" describe --tags --exact-match 2>/dev/null || \
      git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || true
  fi
}

current_git_commit() {
  if [[ -d "$ROOT_DIR/.git" ]] && command -v git >/dev/null 2>&1; then
    git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || true
  fi
}

release_manifest_path() {
  local release_id="$1"
  printf '%s/%s.json\n' "$RELEASE_HISTORY_DIR" "$release_id"
}

init_release_context() {
  [[ -n "$DEPLOY_RELEASE_ID" ]] && return 0
  DEPLOY_RELEASE_ID="$(release_timestamp)"
  DEPLOY_STARTED_AT="$(utc_now)"
  DEPLOY_GIT_REF="$(current_git_ref)"
  DEPLOY_GIT_COMMIT="$(current_git_commit)"
  mkdir -p "$RELEASE_HISTORY_DIR"
  chmod 700 "$RELEASE_STATE_DIR" "$RELEASE_HISTORY_DIR"
  PREVIOUS_API_IMAGE="$(env_value IDEABOARD_API_IMAGE)"
  PREVIOUS_AUTH_IMAGE="$(env_value IDEABOARD_AUTH_IMAGE)"
  PREVIOUS_AUTH_MIGRATE_IMAGE="$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)"
  DEPLOY_ENV_SNAPSHOT="$(mktemp "$RELEASE_STATE_DIR/env.${DEPLOY_RELEASE_ID}.XXXXXX")"
  cp "$ENV_FILE" "$DEPLOY_ENV_SNAPSHOT"
  chmod 600 "$DEPLOY_ENV_SNAPSHOT"
}

write_release_manifest() {
  local status="$1"
  local manifest
  local tmp
  local completed_at=""
  manifest="$(release_manifest_path "$DEPLOY_RELEASE_ID")"
  tmp="${manifest}.tmp"
  if [[ "$status" == "active" || "$status" == "failed" ]]; then
    completed_at="$(utc_now)"
  fi
  cat > "$tmp" <<EOF
{
  "release_id": $(json_value "$DEPLOY_RELEASE_ID"),
  "status": $(json_value "$status"),
  "stage": $(json_value "$DEPLOY_STAGE"),
  "started_at": $(json_value "$DEPLOY_STARTED_AT"),
  "completed_at": $(json_value "$completed_at"),
  "git_ref": $(json_value "$DEPLOY_GIT_REF"),
  "git_commit": $(json_value "$DEPLOY_GIT_COMMIT"),
  "images": {
    "api": $(json_value "${DEPLOY_API_IMAGE:-$(env_value IDEABOARD_API_IMAGE)}"),
    "auth": $(json_value "${DEPLOY_AUTH_IMAGE:-$(env_value IDEABOARD_AUTH_IMAGE)}"),
    "auth_migrate": $(json_value "${DEPLOY_AUTH_MIGRATE_IMAGE:-$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)}")
  },
  "previous_images": {
    "api": $(json_value "$PREVIOUS_API_IMAGE"),
    "auth": $(json_value "$PREVIOUS_AUTH_IMAGE"),
    "auth_migrate": $(json_value "$PREVIOUS_AUTH_MIGRATE_IMAGE")
  },
  "backups": {
    "postgres": $(json_value "$DEPLOY_POSTGRES_BACKUP"),
    "uploads": $(json_value "$DEPLOY_UPLOADS_BACKUP")
  },
  "source_manifest": {
    "source": $(json_value "$DEPLOY_SOURCE_MANIFEST"),
    "local_copy": $(json_value "$DEPLOY_SOURCE_MANIFEST_COPY")
  },
  "gates": {
    "preflight": $(json_value "$DEPLOY_PREFLIGHT_GATE"),
    "cosign": $(json_value "$DEPLOY_COSIGN_GATE"),
    "backup": $(json_value "$DEPLOY_BACKUP_GATE"),
    "migration": $(json_value "$DEPLOY_MIGRATION_GATE"),
    "smoke": $(json_value "$DEPLOY_SMOKE_GATE"),
    "migration_started": $(json_value "$DEPLOY_MIGRATION_STARTED")
  }
}
EOF
  mv "$tmp" "$manifest"
  chmod 600 "$manifest"
  if [[ "$status" == "active" ]]; then
    if [[ -f "$CURRENT_RELEASE_FILE" ]]; then
      cp "$CURRENT_RELEASE_FILE" "$PREVIOUS_RELEASE_FILE"
      chmod 600 "$PREVIOUS_RELEASE_FILE"
    fi
    cp "$manifest" "$CURRENT_RELEASE_FILE"
    chmod 600 "$CURRENT_RELEASE_FILE"
  fi
}

restore_env_snapshot() {
  if [[ -n "$DEPLOY_ENV_SNAPSHOT" && -f "$DEPLOY_ENV_SNAPSHOT" ]]; then
    cp "$DEPLOY_ENV_SNAPSHOT" "$ENV_FILE"
    chmod 600 "$ENV_FILE"
  fi
}

capture_deploy_image_refs() {
  DEPLOY_API_IMAGE="$(env_value IDEABOARD_API_IMAGE)"
  DEPLOY_AUTH_IMAGE="$(env_value IDEABOARD_AUTH_IMAGE)"
  DEPLOY_AUTH_MIGRATE_IMAGE="$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)"
}

print_recovery_hint() {
  local manifest=""
  if [[ -n "$DEPLOY_RELEASE_ID" ]]; then
    manifest="$(release_manifest_path "$DEPLOY_RELEASE_ID")"
  fi
  echo >&2
  echo "Deployment did not complete." >&2
  [[ -n "$manifest" ]] && echo "Release manifest: $manifest" >&2
  [[ -n "$DEPLOY_POSTGRES_BACKUP" ]] && echo "PostgreSQL backup: $DEPLOY_POSTGRES_BACKUP" >&2
  [[ -n "$DEPLOY_UPLOADS_BACKUP" ]] && echo "Uploads backup: $DEPLOY_UPLOADS_BACKUP" >&2
  echo "Previous API image: $PREVIOUS_API_IMAGE" >&2
  echo "Previous Auth image: $PREVIOUS_AUTH_IMAGE" >&2
  echo "Previous Auth migration image: $PREVIOUS_AUTH_MIGRATE_IMAGE" >&2
  echo "Inspect logs: ideaboard logs --tail=200 api auth postgres nats" >&2
  if (( DEPLOY_MIGRATION_STARTED == 0 )); then
    if (( DEPLOY_ENV_RESTORE_ALLOWED == 1 )); then
      echo "Migration did not start; .env was restored to the previous image refs." >&2
    else
      echo "Migration did not start; .env was left at the requested image refs for inspection." >&2
    fi
  else
    echo "Migration may have run; database restore is not automatic. Inspect backups before rolling back DB state." >&2
  fi
}

handle_deploy_failure() {
  local exit_code="$1"
  if [[ "$DEPLOY_TRAP_ENABLED" == "1" && "$DEPLOY_SUCCEEDED" == "0" && "$exit_code" != "0" && -n "$DEPLOY_RELEASE_ID" ]]; then
    if (( DEPLOY_IMAGE_UPDATE_PENDING == 1 && DEPLOY_ENV_RESTORE_ALLOWED == 1 && DEPLOY_MIGRATION_STARTED == 0 )); then
      restore_env_snapshot
    fi
    write_release_manifest "failed" || true
    if [[ -n "$DEPLOY_ENV_SNAPSHOT" ]]; then
      rm -f -- "$DEPLOY_ENV_SNAPSHOT"
    fi
    print_recovery_hint
  fi
  exit "$exit_code"
}

enable_deploy_failure_trap() {
  DEPLOY_TRAP_ENABLED=1
  trap 'handle_deploy_failure "$?"' EXIT
}

complete_deploy_success() {
  DEPLOY_STAGE="complete"
  DEPLOY_SMOKE_GATE="passed"
  write_release_manifest "active"
  finish_trapped_command_success
}

finish_trapped_command_success() {
  if [[ -n "$DEPLOY_ENV_SNAPSHOT" ]]; then
    rm -f -- "$DEPLOY_ENV_SNAPSHOT"
  fi
  DEPLOY_SUCCEEDED=1
  trap - EXIT
}

print_release_file_summary() {
  local label="$1"
  local file="$2"

  echo "$label: $file"
  if [[ ! -f "$file" ]]; then
    echo "  not found"
    return 0
  fi
  sed -n \
    -e 's/^[[:space:]]*"release_id":[[:space:]]*"\([^"]*\)".*/  release_id: \1/p' \
    -e 's/^[[:space:]]*"status":[[:space:]]*"\([^"]*\)".*/  status: \1/p' \
    -e 's/^[[:space:]]*"stage":[[:space:]]*"\([^"]*\)".*/  stage: \1/p' \
    -e 's/^[[:space:]]*"started_at":[[:space:]]*"\([^"]*\)".*/  started_at: \1/p' \
    -e 's/^[[:space:]]*"completed_at":[[:space:]]*"\([^"]*\)".*/  completed_at: \1/p' \
    -e 's/^[[:space:]]*"postgres":[[:space:]]*"\([^"]*\)".*/  backup.postgres: \1/p' \
    -e 's/^[[:space:]]*"uploads":[[:space:]]*"\([^"]*\)".*/  backup.uploads: \1/p' \
    -e 's/^[[:space:]]*"backup":[[:space:]]*"\([^"]*\)".*/  gate.backup: \1/p' \
    "$file"
}

print_release_summary() {
  echo "Release state directory: $RELEASE_STATE_DIR"
  print_release_file_summary "Current release" "$CURRENT_RELEASE_FILE"
  print_release_file_summary "Previous release" "$PREVIOUS_RELEASE_FILE"
}

print_env_image_refs() {
  echo "Configured image refs:"
  echo "  api: $(env_value IDEABOARD_API_IMAGE)"
  echo "  auth: $(env_value IDEABOARD_AUTH_IMAGE)"
  echo "  auth-migrate: $(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)"
}

print_recent_backups() {
  local backup
  local encrypted
  local checksum

  echo "Recent backups: $BACKUP_DIR"
  if [[ ! -d "$BACKUP_DIR" ]]; then
    echo "  not found"
    return 0
  fi
  find "$BACKUP_DIR" -maxdepth 1 -type f \
    \( -name 'postgres-*.dump' -o -name 'postgres-*.dump.age' -o -name 'postgres-*.dump.gpg' -o -name 'api-uploads-*.tar.gz' -o -name 'api-uploads-*.tar.gz.age' -o -name 'api-uploads-*.tar.gz.gpg' \) \
    ! -name '*.sha256' \
    | sort \
    | tail -n 10 \
    | while IFS= read -r backup; do
      encrypted="no"
      checksum="$(backup_checksum_status "$backup")"
      [[ "$backup" == *.age || "$backup" == *.gpg ]] && encrypted="yes"
      printf '  %s checksum=%s encrypted=%s\n' "$backup" "$checksum" "$encrypted"
    done
}

run_doctor() {
  local mode="${1:-}"

  print_release_summary
  print_env_image_refs
  echo
  echo "Compose services:"
  docker_compose ps || true
  echo
  print_recent_backups

  if [[ "$mode" == "--deep" ]]; then
    echo
    echo "Deep health checks:"
    run_backend_smoke_checks
    local base_url
    base_url="$(env_value API_BASE_URL)"
    base_url="${base_url%/}"
    if [[ -n "$base_url" ]]; then
      echo "Checking public auth session route: ${base_url}/api/auth/get-session"
      http_reaches_server "${base_url}/api/auth/get-session" || die "public auth session route check failed"
    fi
  fi
}

fetch_update_manifest() {
  local source="$1"
  local dest

  [[ -n "$source" ]] || die "manifest source is required"
  dest="$(mktemp "$RELEASE_STATE_DIR/source-manifest.${DEPLOY_RELEASE_ID}.XXXXXX")"
  case "$source" in
    http://*|https://*)
      if command -v curl >/dev/null 2>&1; then
        curl -fsSL --connect-timeout 10 --max-time 60 "$source" -o "$dest"
      elif command -v wget >/dev/null 2>&1; then
        wget -q -T 60 -O "$dest" "$source"
      else
        die "curl or wget is required to fetch release manifest URL"
      fi
      ;;
    *)
      [[ -r "$source" ]] || die "manifest file is not readable: $source"
      cp "$source" "$dest"
      ;;
  esac
  chmod 600 "$dest"
  printf '%s\n' "$dest"
}

apply_update_manifest() {
  local manifest_file="$1"
  local api_image
  local auth_image
  local auth_migrate_image
  local tag
  local commit

  api_image="$(json_image_field "api" "$manifest_file")"
  auth_image="$(json_image_field "auth" "$manifest_file")"
  auth_migrate_image="$(json_image_field "auth-migrate" "$manifest_file")"
  if [[ -z "$auth_migrate_image" ]]; then
    auth_migrate_image="$(json_image_field "auth_migrate" "$manifest_file")"
  fi
  [[ -n "$api_image" && -n "$auth_image" && -n "$auth_migrate_image" ]] || \
    die "release manifest must contain images.api, images.auth, and images.auth-migrate"
  is_ideaboard_image_ref "$api_image" "api" || die "release manifest has invalid api image ref"
  is_ideaboard_image_ref "$auth_image" "auth" || die "release manifest has invalid auth image ref"
  is_ideaboard_image_ref "$auth_migrate_image" "auth-migrate" || die "release manifest has invalid auth-migrate image ref"

  tag="$(json_string_field "tag" "$manifest_file")"
  commit="$(json_string_field "commit" "$manifest_file")"
  echo "Applying release manifest${tag:+ tag: $tag}${commit:+ commit: $commit}"
  set_env_value IDEABOARD_API_IMAGE "$api_image"
  set_env_value IDEABOARD_AUTH_IMAGE "$auth_image"
  set_env_value IDEABOARD_AUTH_MIGRATE_IMAGE "$auth_migrate_image"
  DEPLOY_IMAGE_UPDATE_PENDING=1
}

needs_auto_update() {
  [[ "$is_update" == "1" ]] || \
    is_placeholder "$(env_value IDEABOARD_API_IMAGE)" || \
    is_placeholder "$(env_value IDEABOARD_AUTH_IMAGE)" || \
    is_placeholder "$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)" || \
    ! is_ideaboard_image_ref "$(env_value IDEABOARD_API_IMAGE)" "api" || \
    ! is_ideaboard_image_ref "$(env_value IDEABOARD_AUTH_IMAGE)" "auth" || \
    ! is_ideaboard_image_ref "$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)" "auth-migrate"
}

resolve_deploy_images() {
  if [[ -n "$update_manifest_source" ]]; then
    local manifest_file
    manifest_file="$(fetch_update_manifest "$update_manifest_source")"
    DEPLOY_SOURCE_MANIFEST="$update_manifest_source"
    DEPLOY_SOURCE_MANIFEST_COPY="$manifest_file"
    apply_update_manifest "$manifest_file"
    return 0
  fi

  if needs_auto_update; then
    perform_auto_update
  fi
}

run_deploy_transaction() {
  init_release_context
  enable_deploy_failure_trap

  DEPLOY_STAGE="resolve-images"
  resolve_deploy_images
  capture_deploy_image_refs

  DEPLOY_STAGE="preflight"
  preflight_checks

  DEPLOY_STAGE="pull"
  prepare_stack_images

  DEPLOY_STAGE="start-postgres"
  docker_compose up -d postgres
  wait_for_postgres

  DEPLOY_STAGE="backup"
  backup_postgres
  backup_api_uploads
  DEPLOY_BACKUP_GATE="passed"

  DEPLOY_STAGE="api-migration"
  DEPLOY_MIGRATION_STARTED=1
  run_api_migration

  DEPLOY_STAGE="auth-migration"
  run_auth_migration
  DEPLOY_MIGRATION_GATE="passed"
  DEPLOY_ENV_RESTORE_ALLOWED=0

  DEPLOY_STAGE="up"
  docker_compose up -d

  DEPLOY_STAGE="smoke"
  run_backend_smoke_checks

  complete_deploy_success
  docker_compose ps
}

restore_previous_image_refs() {
  [[ -f "$PREVIOUS_RELEASE_FILE" ]] || die "previous release manifest not found: $PREVIOUS_RELEASE_FILE"
  local api_image
  local auth_image
  local auth_migrate_image
  api_image="$(json_object_string_field "images" "api" "$PREVIOUS_RELEASE_FILE")"
  auth_image="$(json_object_string_field "images" "auth" "$PREVIOUS_RELEASE_FILE")"
  auth_migrate_image="$(json_object_string_field "images" "auth_migrate" "$PREVIOUS_RELEASE_FILE")"
  if [[ -z "$auth_migrate_image" ]]; then
    auth_migrate_image="$(json_object_string_field "images" "auth-migrate" "$PREVIOUS_RELEASE_FILE")"
  fi
  [[ -n "$api_image" && -n "$auth_image" && -n "$auth_migrate_image" ]] || die "previous release manifest is missing image refs"
  is_ideaboard_image_ref "$api_image" "api" || die "previous release manifest has invalid api image ref"
  is_ideaboard_image_ref "$auth_image" "auth" || die "previous release manifest has invalid auth image ref"
  is_ideaboard_image_ref "$auth_migrate_image" "auth-migrate" || die "previous release manifest has invalid auth-migrate image ref"
  set_env_value IDEABOARD_API_IMAGE "$api_image"
  set_env_value IDEABOARD_AUTH_IMAGE "$auth_image"
  set_env_value IDEABOARD_AUTH_MIGRATE_IMAGE "$auth_migrate_image"
}

run_previous_rollback() {
  echo "Restoring previous image refs from $PREVIOUS_RELEASE_FILE"
  init_release_context
  DEPLOY_IMAGE_UPDATE_PENDING=1
  enable_deploy_failure_trap

  DEPLOY_STAGE="rollback-restore-env"
  restore_previous_image_refs
  capture_deploy_image_refs

  DEPLOY_STAGE="preflight"
  preflight_checks

  DEPLOY_STAGE="pull"
  prepare_stack_images

  DEPLOY_STAGE="up"
  docker_compose up -d
  DEPLOY_ENV_RESTORE_ALLOWED=0

  DEPLOY_STAGE="smoke"
  run_backend_smoke_checks

  complete_deploy_success
  docker_compose ps
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

install_cosign() {
  echo "cosign is missing. Attempting to install it automatically..." >&2

  local os
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  local arch
  arch=$(uname -m)
  if [[ "$arch" == "x86_64" ]]; then
    arch="amd64"
  elif [[ "$arch" == "aarch64" || "$arch" == "arm64" ]]; then
    arch="arm64"
  else
    echo "Warning: unsupported architecture $arch for auto-installing cosign." >&2
    return 1
  fi

  local version="v2.4.1"
  local url="https://github.com/sigstore/cosign/releases/download/${version}/cosign-${os}-${arch}"
  local dest="/usr/local/bin/cosign"

  echo "Downloading cosign from $url..." >&2
  if curl -fsSL "$url" -o "$dest"; then
    chmod +x "$dest"
    echo "cosign has been successfully installed to $dest" >&2
    return 0
  fi

  # Fallback to local project bin if /usr/local/bin is not writable
  local local_bin="$ROOT_DIR/bin"
  mkdir -p "$local_bin"
  dest="$local_bin/cosign"
  echo "Retrying download to local bin: $dest..." >&2
  if curl -fsSL "$url" -o "$dest"; then
    chmod +x "$dest"
    export PATH="$local_bin:$PATH"
    echo "cosign has been successfully installed to $dest and added to PATH" >&2
    return 0
  fi

  echo "Error: failed to download cosign automatically." >&2
  return 1
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
  if ! command -v cosign >/dev/null 2>&1; then
    if ! install_cosign; then
      die "cosign is required to verify image signatures. Install cosign or set COSIGN_VERIFY=false only for temporary offline recovery."
    fi
  fi

  identity_regexp="$(env_value COSIGN_CERTIFICATE_IDENTITY_REGEXP)"
  if [[ -z "$identity_regexp" ]]; then
    source_repo="$(image_source_repo_from_ref "$image_ref")" || die "cannot derive GitHub source repository from image ref: $image_ref"
    source_repo_regexp="${source_repo//./\\.}"
    identity_regexp="(?i)https://github.com/${source_repo_regexp}/\\.github/workflows/docker-images\\.yml@refs/(heads|tags)/.+"
  fi
  oidc_issuer="$(env_value COSIGN_CERTIFICATE_OIDC_ISSUER)"
  oidc_issuer="${oidc_issuer:-https://token.actions.githubusercontent.com}"

  echo "Verifying image signature: $image_ref"
  if ! cosign verify \
    --certificate-identity-regexp "$identity_regexp" \
    --certificate-oidc-issuer "$oidc_issuer" \
    "$image_ref" >/dev/null; then
    echo "Error: Image signature verification failed for $image_ref." >&2
    echo "If you built these images locally (not via GitHub Actions) or want to bypass verification, please set COSIGN_VERIFY=false in /opt/stacks/ideaboard/docker/.env" >&2
    return 1
  fi
}

verify_image_signatures() {
  verify_image_signature "$(env_value IDEABOARD_API_IMAGE)"
  verify_image_signature "$(env_value IDEABOARD_AUTH_IMAGE)"
  verify_image_signature "$(env_value IDEABOARD_AUTH_MIGRATE_IMAGE)"
  DEPLOY_COSIGN_GATE="passed"
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
    NGINX_SERVER_NAME="${NGINX_SERVER_NAME:-$(env_or_file_value NGINX_SERVER_NAME)}" \
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
  DEPLOY_IMAGE_UPDATE_PENDING=1
  DEPLOY_GIT_REF="$(current_git_ref)"
  DEPLOY_GIT_COMMIT="$(current_git_commit)"

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

case "$command" in
  deploy)
    run_deploy_transaction
    ;;
  smoke)
    validate_env_file || exit 1
    run_backend_smoke_checks
    docker_compose ps
    ;;
  restart)
    validate_env_file || exit 1
    docker_compose restart
    docker_compose ps
    ;;
  stop)
    validate_env_file || exit 1
    docker_compose stop
    ;;
  status)
    validate_env_file || exit 1
    print_release_summary
    print_env_image_refs
    echo
    docker_compose ps
    ;;
  doctor)
    validate_env_file || exit 1
    shift || true
    run_doctor "${1:-}"
    ;;
  preflight)
    preflight_checks
    ;;
  rollback)
    validate_env_file || exit 1
    shift || true
    if [[ "${1:-}" != "--to" || "${2:-}" != "previous" || -n "${3:-}" ]]; then
      die "usage: ideaboard rollback --to previous"
    fi
    run_previous_rollback
    ;;
  logs)
    validate_env_file || exit 1
    shift || true
    docker_compose logs -f "$@"
    ;;
  pull)
    validate_env_file || exit 1
    prepare_stack_images
    ;;
  up)
    validate_env_file || exit 1
    docker_compose up -d
    ;;
  migrate)
    validate_env_file || exit 1
    init_release_context
    enable_deploy_failure_trap
    DEPLOY_STAGE="pull"
    prepare_stack_images
    DEPLOY_STAGE="start-postgres"
    docker_compose up -d postgres
    wait_for_postgres
    DEPLOY_STAGE="backup"
    backup_postgres
    backup_api_uploads
    DEPLOY_BACKUP_GATE="passed"
    DEPLOY_STAGE="api-migration"
    DEPLOY_MIGRATION_STARTED=1
    run_api_migration
    DEPLOY_STAGE="auth-migration"
    run_auth_migration
    DEPLOY_MIGRATION_GATE="passed"
    finish_trapped_command_success
    ;;
  ps)
    validate_env_file || exit 1
    docker_compose ps
    ;;
  *)
    cat >&2 <<EOF
Usage: ideaboard [update|deploy|preflight|doctor|smoke|restart|stop|status|logs|delete|uninstall|init-env|pull|up|migrate|rollback|ps]
       ideaboard update [--manifest path-or-url]
       ideaboard doctor [--deep]
       ideaboard rollback --to previous
       ideaboard nginx-conf [output-path]
       ideaboard cloudflare-real-ip-conf [output-path]

Default command: update
Environment file: $ENV_FILE
EOF
    exit 1
    ;;
esac
