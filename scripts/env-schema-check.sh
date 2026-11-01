#!/usr/bin/env bash
# env-schema-check.sh
#
# P0 env drift gate: the single source of truth for env names is code
# (Go `env:"..."` tags + Node `getEnv[source], '...'` reads + the
# `problems.push` / `missing = append(missing, ...)` required lists, plus
# the Worker Env binding interface).
# Every `.env.example` is derived documentation and must track it:
#
#   A. required schema keys are documented in the owning .env.example
#   B. every key in a .env.example is known (schema, compose, or allowlist)
#   C. removed legacy names appear in no tracked file
#   D. active example values are placeholders, never usable-looking secrets
#   E. every compose environment name is backed by app schema or an
#      explicit third-party/deploy allowlist entry
#   F. the dotenvx-encrypted docker/.env (when present) carries known key
#      names only and never a DOTENV_PRIVATE_KEY; values are opaque
#      ciphertext and are intentionally not reviewed
#
# Usage: ./scripts/env-schema-check.sh [repo-root]
# Exit 0 when all checks pass, 1 with a failure list otherwise.
#
# Portable to bash 3.2 (macOS /usr/bin/bash): newline-separated key lists,
# no associative arrays, no empty-array expansions.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck source=scripts/lib/env-schema.sh
source "$SCRIPT_DIR/lib/env-schema.sh"

ROOT_DIR="${1:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

GO_CONFIG="$ROOT_DIR/apps/api/internal/platform/config/config.go"
AUTH_ENV="$ROOT_DIR/apps/auth/src/infrastructure/env.ts"
RT_ENV="$ROOT_DIR/apps/realtime/src/infrastructure/env.ts"
WORKER_ENV="$ROOT_DIR/apps/ai-proxy/src/contracts.ts"
WORKER_EXAMPLE="$ROOT_DIR/apps/ai-proxy/.dev.vars.example"
API_EXAMPLE="$ROOT_DIR/apps/api/.env.example"
AUTH_EXAMPLE="$ROOT_DIR/apps/auth/.env.example"
RT_EXAMPLE="$ROOT_DIR/apps/realtime/.env.example"
DOCKER_EXAMPLE="$ROOT_DIR/docker/.env.example"
IMAGES_EXAMPLE="$ROOT_DIR/docker/images.env.example"
DOCKER_ENV="$ROOT_DIR/docker/.env"
MIGRATION_EXAMPLE="$ROOT_DIR/docker/migration.env.example"
COMPOSE="$ROOT_DIR/docker/compose.yml"
COMPOSE_LOCAL="$ROOT_DIR/docker/compose.local.yml"
INIT_ROLES="$ROOT_DIR/docker/postgres/init-runtime-roles.sh"

failures=0
fail() {
  printf 'env-schema-check: %s\n' "$1" >&2
  failures=$((failures + 1))
}

# Required compose inputs (${NAME:?}).
# shellcheck disable=SC2016 # Match literal Compose interpolation, not shell variables.
compose_required() { grep -oh '\${[A-Z][A-Z0-9_]*:?' "$1" | tr -d '${:?' | sort -u; }

# Literal container env names (left side of `NAME:` under environment:).
compose_env_names() { grep -ohE '^      [A-Z][A-Z0-9_]*:' "$1" | tr -d ' :' | sort -u; }

# Removed legacy names: a hit in any tracked file fails the gate so renames
# can never silently re-enter through a stale copy-paste.
legacy_names() {
  cat <<'EOF'
API_DB_PASSWORD
AUTH_DB_PASSWORD
AUTH_SERVICE_BASE_URL
DB_HOST
DB_MIGRATION_PASSWORD
DB_MIGRATION_USER
DB_NAME
DB_PASSWORD
DB_PORT
DB_SSLMODE
DB_USER
REALTIME_API_INTERNAL_URL
REALTIME_HOST
REALTIME_HOST_PORT
REALTIME_INTERNAL_KEY
REALTIME_PORT
REALTIME_TICKET_SECRET
REALTIME_Y_COMPACT_EVERY
REALTIME_Y_PERSIST_RETRIES
EOF
}


# --- check A: required keys are documented ------------------------------------

check_required_documented() {
  local owner="$1"
  local example="$2"
  local required="$3"
  local documented
  documented="$(example_keys "$example")"
  local key
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$documented"; then
      fail "A: required $key ($owner) is not documented in $example"
    fi
  done <<<"$required"
}

# --- check B: example keys are known -------------------------------------------

check_example_known() {
  local example="$1"
  local known="$2"
  local key
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$known"; then
      fail "B: $example documents unknown key $key"
    fi
  done < <(example_keys "$example")
}

# --- check C: legacy names are gone ----------------------------------------------

check_no_legacy() {
  local pattern
  pattern="$(legacy_names | paste -sd'|' -)"
  local hits
  if git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    # The gate script and its self-test carry the legacy blocklist and
    # legacy fixtures, so both are excluded from their own scan; generated
    # API docs are covered because they are regenerated from the contract
    # sources above.
    # -w matches whole words only, so longer names that merely contain a
    # legacy name (VITE_API_BASE_URL, QA_API_BASE_URL, DB_HOSTNAME, ...)
    # are not flagged. -o prints file:match (no line number), so the match
    # itself is field 2.
    # Frozen ADR history is exempt by file: docs/adr/0019-realtime-presence.md
    # keeps the pre-rename REALTIME_TICKET_SECRET in its append-only frozen
    # body, with the rename recorded in its Status appendix (2026-10-20).
    # Live config still fails on any legacy name; only that file is skipped.
    hits="$(git -C "$ROOT_DIR" grep -E -w -o --no-color "$pattern" -- . ':!scripts/env-schema-check.sh' ':!scripts/env-schema-check-test.sh' ':!docs/adr/0019-realtime-presence.md' 2>/dev/null | cut -d: -f2- | sort -u || true)"
  else
    # Frozen ADR history (docs/adr/0019-realtime-presence.md) is exempt in
    # both branches: its append-only frozen body keeps the pre-rename name.
    hits="$(grep -rwE -oh "$pattern" "$ROOT_DIR" --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=dist --exclude=0019-realtime-presence.md 2>/dev/null | sort -u || true)"
  fi
  local hit
  while IFS= read -r hit; do
    [[ -z "$hit" ]] && continue
    fail "C: legacy env name still referenced: $hit"
  done <<<"$hits"
  # Untracked-but-canonical example files (e.g. a newly added .env.example
  # awaiting git add) are scanned directly since git grep skips them.
  local doc
  for doc in "$WORKER_EXAMPLE" "$API_EXAMPLE" "$AUTH_EXAMPLE" "$RT_EXAMPLE" "$DOCKER_EXAMPLE" "$IMAGES_EXAMPLE" "$MIGRATION_EXAMPLE" "$COMPOSE" "$COMPOSE_LOCAL"; do
    [[ -f "$doc" ]] || continue
    while IFS= read -r hit; do
      [[ -z "$hit" ]] && continue
      fail "C: legacy env name still referenced in $doc: $hit"
    done < <(grep -E -wo "$pattern" "$doc" 2>/dev/null | sort -u || true)
  done
}

# --- check D: no usable-looking secrets in examples ------------------------------

check_no_weak_example_values() {
  local file
  for file in "$WORKER_EXAMPLE" "$API_EXAMPLE" "$AUTH_EXAMPLE" "$RT_EXAMPLE" "$DOCKER_EXAMPLE" "$IMAGES_EXAMPLE" "$MIGRATION_EXAMPLE"; do
    [[ -f "$file" ]] || { fail "D: missing example file $file"; continue; }
    while IFS= read -r line; do
      [[ "$line" =~ ^[[:space:]]*# ]] && continue
      [[ "$line" != *=* ]] && continue
      local key="${line%%=*}"
      local value="${line#*=}"
      key="$(printf '%s' "$key" | tr -d '[:space:]')"
      [[ "$key" =~ ^[A-Z][A-Z0-9_]*$ ]] || continue
      local lower
      lower="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
      if [[ "$lower" == *ideaboard123* || "$lower" == changeme || "$lower" == password || "$lower" == secret ]]; then
        fail "D: $file sets weak value for $key"
      elif [[ "$value" =~ ^[0-9a-fA-F]{64}$ ]]; then
        fail "D: $file sets hex-secret-looking value for $key (use a replace-with placeholder)"
      elif [[ "$value" =~ ^[A-Za-z0-9+/]{40,}={0,2}$ ]]; then
        fail "D: $file sets base64-secret-looking value for $key (use a replace-with placeholder)"
      fi
    done <"$file"
  done
}

# --- check E: compose names are backed ------------------------------------------

check_compose_backed() {
  local known="$1"
  local file
  for file in "$COMPOSE" "$COMPOSE_LOCAL"; do
    local name
    while IFS= read -r name; do
      [[ -z "$name" ]] && continue
      if ! contains_key "$name" "$known"; then
        fail "E: $file sets unknown container env $name"
      fi
    done < <(compose_env_names "$file")
  done
}

# --- check F: encrypted docker/.env carries key names only -----------------------
#
# The committed docker/.env is dotenvx-encrypted: values are opaque
# ciphertext (never reviewed, never weakened by check D), but every key name
# in a secret-change PR is visible and must be known. The private key lives
# only in a password manager (exported at use time, never stored on disk), never in this file.

check_encrypted_env_keys() {
  local known="$1"
  [[ -f "$DOCKER_ENV" ]] || return 0
  if grep -q 'DOTENV_PRIVATE_KEY' "$DOCKER_ENV"; then
    fail "F: $DOCKER_ENV must never contain DOTENV_PRIVATE_KEY (keep it in a password manager, export at use time)"
  fi
  local key
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if [[ "$key" == IDEABOARD_*_IMAGE ]]; then
      fail "F: $DOCKER_ENV must not store image refs (use docker/images.env)"
    fi
    # RIVER_WORKER_CONCURRENCY is a plaintext deploy input (non-secret):
    # it lives in docker/.env.example and the deploy process env, never in
    # the encrypted file, so concurrency tweaks do not churn ciphertext.
    if [[ "$key" == RIVER_WORKER_CONCURRENCY ]]; then
      fail "F: $DOCKER_ENV must not store plaintext RIVER_WORKER_CONCURRENCY (use docker/.env.example or deployment process env)"
    fi
    if ! contains_key "$key" "$(printf '%s\nDOTENV_PUBLIC_KEY' "$known")"; then
      fail "F: $DOCKER_ENV sets unknown key $key"
    fi
  done < <(example_keys "$DOCKER_ENV")
}

main() {
  local file
  for file in "$GO_CONFIG" "$AUTH_ENV" "$RT_ENV" "$WORKER_ENV" "$WORKER_EXAMPLE" "$API_EXAMPLE" "$AUTH_EXAMPLE" "$RT_EXAMPLE" "$DOCKER_EXAMPLE" "$IMAGES_EXAMPLE" "$MIGRATION_EXAMPLE" "$COMPOSE" "$COMPOSE_LOCAL"; do
    [[ -f "$file" ]] || fail "missing input $file"
  done
  (( failures > 0 )) && return 1

  local go_keys go_req auth_keys auth_req rt_keys rt_req worker_keys
  local deploy_keys third_party command_keys all_known
  go_keys="$(go_schema "$GO_CONFIG")"
  go_req="$(go_required "$GO_CONFIG")"
  auth_keys="$(node_schema "$AUTH_ENV")"
  auth_req="$(node_required "$AUTH_ENV")"
  rt_keys="$(node_schema "$RT_ENV")"
  rt_req="$(node_required "$RT_ENV")"
  worker_keys="$(worker_schema "$WORKER_ENV")"
  [[ -n "$worker_keys" ]] || fail "Worker Env binding schema is empty"
  deploy_keys="$(deploy_allowlist)"
  third_party="$(third_party_allowlist)"
  command_keys="$(command_allowlist)"
  all_known="$(printf '%s\n%s\n%s\n%s\n%s\n%s\n' "$go_keys" "$auth_keys" "$rt_keys" "$deploy_keys" "$third_party" "$command_keys" | sort -u)"

  # A: required schema keys documented in the owning example.
  check_required_documented "go ValidateRequired" "$API_EXAMPLE" "$go_req"
  check_required_documented "auth loadEnv" "$AUTH_EXAMPLE" "$auth_req"
  check_required_documented "realtime loadEnv" "$RT_EXAMPLE" "$rt_req"
  check_required_documented "Worker Env" "$WORKER_EXAMPLE" "$worker_keys"
  # Compose-required inputs must be settable from docker/.env.
  local compose_req docker_documented
  compose_req="$(printf '%s\n%s\n' "$(compose_required "$COMPOSE")" "$(compose_required "$COMPOSE_LOCAL")" | sort -u)"
  docker_documented="$(printf '%s\n%s\n%s\n' "$(example_keys "$DOCKER_EXAMPLE")" "$(example_keys "$IMAGES_EXAMPLE")" "$third_party" | sort -u)"
  local key
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$docker_documented"; then
      fail "A: compose-required $key is not documented in $DOCKER_EXAMPLE"
    fi
  done <<<"$compose_req"
  # Migration example carries the full PG connection surface for the owner role.
  check_required_documented "migration" "$MIGRATION_EXAMPLE" "$(printf 'PG_HOST\nPG_PORT\nPG_DB\nPG_SSLMODE\nPG_MIGRATION_USER\nPG_MIGRATION_PASSWORD\n')"

  # B: example keys are known.
  # Edge-only bindings belong to the Worker example, not VPS/Compose inputs.
  check_example_known "$WORKER_EXAMPLE" "$worker_keys"
  check_example_known "$API_EXAMPLE" "$all_known"
  check_example_known "$AUTH_EXAMPLE" "$all_known"
  check_example_known "$RT_EXAMPLE" "$all_known"
  check_example_known "$DOCKER_EXAMPLE" "$all_known"
  check_example_known "$IMAGES_EXAMPLE" "$deploy_keys"
  check_example_known "$MIGRATION_EXAMPLE" "$all_known"

  # C + D + F.
  check_no_legacy
  check_no_weak_example_values
  check_encrypted_env_keys "$all_known"

  # E: compose container env names backed by a schema or allowlist.
  # The postgres init script owns the role-password relay vars.
  local init_vars
  init_vars="$(grep -oE '\$[A-Z][A-Z0-9_]*' "$INIT_ROLES" | tr -d '$' | sort -u || true)"
  check_compose_backed "$(printf '%s\n%s\nPOSTGRES_USER\nPOSTGRES_DB\n' "$all_known" "$init_vars" | sort -u)"

  if (( failures > 0 )); then
    printf 'env-schema-check: %d problem(s) found\n' "$failures" >&2
    return 1
  fi
  echo "env schema and examples are in sync"
}

main "$@"
