#!/usr/bin/env bash
# infisical-drift-check.sh
#
# Compares an Infisical dotenv export against the code-owned env schema
# (same sources as scripts/env-schema-check.sh: Go `env:"..."` tags +
# Node getEnv reads + Worker Env bindings). Key NAMES only; values are
# never inspected.
#
# Usage (Infisical Cloud export — hits the Cloud API, needs network + auth):
#   infisical export --domain https://app.infisical.com --projectId <id> \
#     --env=prod --path / --format=dotenv > /tmp/infisical-root.env
#   infisical export --domain https://app.infisical.com --projectId <id> \
#     --env=prod --path /ai-proxy --format=dotenv > /tmp/infisical-ai-proxy.env
#   scripts/infisical-drift-check.sh /tmp/infisical-root.env [repo-root]
#   scripts/infisical-drift-check.sh /tmp/infisical-root.env [repo-root] \
#     --worker-export /tmp/infisical-ai-proxy.env
#   (--worker-export=<file> is accepted too; positional $1=export,
#   $2=repo-root stay unchanged for backward compatibility.)
#
# The checker itself stays offline/hermetic: it only reads the export file
# you hand it, never the network. CI/VPS can export with a read-only service
# token via INFISICAL_TOKEN (see docs/runbooks/secrets-infisical.md §4).
#
# Gate decision (see docs/runbooks/secrets-infisical.md §5): files stay the
# authority — env-schema-check.sh keeps validating code vs *.env.example vs
# compose. This script only checks that the Infisical MIRROR carries every
# required key and no unknown key, so a rotation/migration gap fails loudly
# instead of surfacing as a runtime crash.
#
# Portable to bash 3.2 (macOS /usr/bin/bash).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
# shellcheck source=scripts/lib/env-schema.sh
source "$SCRIPT_DIR/lib/env-schema.sh"

usage() {
  echo "usage: $0 <infisical-export-dotenv-file> [repo-root] [--worker-export <worker-export-dotenv-file>]" >&2
  exit 2
}

positionals=()
WORKER_EXPORT=""
WORKER_EXPORT_GIVEN=0
while (($# > 0)); do
  case "$1" in
    --worker-export=*)
      WORKER_EXPORT="${1#--worker-export=}"
      WORKER_EXPORT_GIVEN=1
      shift
      ;;
    --worker-export)
      WORKER_EXPORT_GIVEN=1
      (($# >= 2)) || usage
      WORKER_EXPORT="$2"
      shift 2
      ;;
    --)
      shift
      positionals+=("$@")
      break
      ;;
    -*)
      usage
      ;;
    *)
      positionals+=("$1")
      shift
      ;;
  esac
done
(( ${#positionals[@]} >= 1 && ${#positionals[@]} <= 2 )) || usage
EXPORT_FILE="${positionals[0]}"
ROOT_DIR="${positionals[1]:-}"
if [[ -z "$ROOT_DIR" ]]; then
  ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

[[ -f "$EXPORT_FILE" ]] || usage
if ((WORKER_EXPORT_GIVEN == 1)) && [[ -z "$WORKER_EXPORT" || ! -f "$WORKER_EXPORT" ]]; then
  usage
fi

GO_CONFIG="$ROOT_DIR/apps/api/internal/platform/config/config.go"
AUTH_ENV="$ROOT_DIR/apps/auth/src/infrastructure/env.ts"
RT_ENV="$ROOT_DIR/apps/realtime/src/infrastructure/env.ts"
WORKER_ENV="$ROOT_DIR/apps/ai-proxy/src/contracts.ts"

for f in "$GO_CONFIG" "$AUTH_ENV" "$RT_ENV" "$WORKER_ENV"; do
  if [[ ! -f "$f" ]]; then
    echo "infisical-drift-check: missing input $f" >&2
    exit 1
  fi
done

failures=0
fail() {
  printf 'infisical-drift-check: %s\n' "$1" >&2
  failures=$((failures + 1))
}

export_keys() { grep -oE '^[[:space:]]*[A-Z][A-Z0-9_]*=' "$1" | grep -oE '[A-Z][A-Z0-9_]*' | sort -u || true; }

go_keys="$(go_schema "$GO_CONFIG")"
go_req="$(go_required "$GO_CONFIG")"
auth_keys="$(node_schema "$AUTH_ENV")"
auth_req="$(node_required "$AUTH_ENV")"
rt_keys="$(node_schema "$RT_ENV")"
rt_req="$(node_required "$RT_ENV")"
worker_keys="$(worker_binding_keys "$WORKER_ENV" worker)"
worker_path="$(worker_binding_source_path)"
if [[ -z "$worker_keys" ]]; then
  echo "infisical-drift-check: Worker Env binding schema is empty" >&2
  exit 1
fi
all_app_keys="$(printf '%s\n%s\n%s\n%s\n' "$go_keys" "$auth_keys" "$rt_keys" "$worker_keys" | sort -u)"
# Worker availability is checked by its local/deployment setup, not by VPS
# bootstrap. Its bindings are recognized without making them VPS-required.
all_required="$(printf '%s\n%s\n%s\n' "$go_req" "$auth_req" "$rt_req" | sort -u)"
exported="$(export_keys "$EXPORT_FILE")"
non_infisical="$(printf '%s\n%s\n%s\n' "$(deploy_allowlist)" "$(third_party_allowlist)" "$(command_allowlist)" | sort -u)"

# 1. Every code-required key must exist in Infisical.
while IFS= read -r key; do
  [[ -z "$key" ]] && continue
  if ! contains_key "$key" "$exported"; then
    fail "required key $key is missing from Infisical export $EXPORT_FILE"
  fi
done <<<"$all_required"

# 2. Every Infisical key must be known (app schema) — deploy-only names must
# never be stored in Infisical; image refs stay in docker/images.env.
while IFS= read -r key; do
  [[ -z "$key" ]] && continue
  case "$key" in
    POSTGRES_PASSWORD | PG_MIGRATION_USER | PG_MIGRATION_PASSWORD)
      fail "owner credential $key must not live in Infisical (use local deployment credentials)"
      continue
      ;;
  esac
  if contains_key "$key" "$all_app_keys"; then
    continue
  fi
  if contains_key "$key" "$non_infisical"; then
    fail "deploy-only key $key must not live in Infisical (use docker/images.env or deployment process env)"
    continue
  fi
  fail "Infisical export carries unknown key $key (not in code schema)"
done <<<"$exported"

# Dual-path assertions (only when --worker-export is given; key NAMES only).
# The /ai-proxy folder is a separate Infisical path, invisible to a root-only
# export, so without these the Worker bindings would be a blind spot.
if ((WORKER_EXPORT_GIVEN == 1)); then
  worker_exported="$(export_keys "$WORKER_EXPORT")"

  # a. Every Worker Env binding must exist in the worker export.
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$worker_exported"; then
      fail "Worker binding $key is missing from worker export $WORKER_EXPORT"
    fi
  done <<<"$worker_keys"

  # b. Every worker-export key must be a Worker binding.
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$worker_keys"; then
      fail "worker export carries unknown key $key (not a Worker Env binding in $WORKER_ENV)"
    fi
  done <<<"$worker_exported"

  # c. Root and worker exports must not overlap (a stale root copy would
  # drift from the /ai-proxy source of truth). NOTE: root still carries a
  # WORKER_INTERNAL_SECRET copy today — this check intentionally fails loud
  # until the dashboard copy is deleted.
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if contains_key "$key" "$worker_exported"; then
      fail "key $key present in both root export and worker export (root / and $worker_path must not overlap; delete the stale root copy)"
    fi
  done <<<"$exported"
fi

if ((failures > 0)); then
  printf 'infisical-drift-check: %d problem(s) found\n' "$failures" >&2
  exit 1
fi
if ((WORKER_EXPORT_GIVEN == 1)); then
  echo "infisical exports match code schema (root + $worker_path)"
else
  echo "infisical export matches code schema"
fi
