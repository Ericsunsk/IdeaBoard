#!/usr/bin/env bash
# infisical-drift-check.sh
#
# Compares an Infisical dotenv export against the code-owned env schema
# (same sources as scripts/env-schema-check.sh: Go `env:"..."` tags +
# Node getEnv reads). Key NAMES only; values are never inspected.
#
# Usage (Infisical Cloud export — hits the Cloud API, needs network + auth):
#   infisical export --domain https://app.infisical.com --projectId <id> \
#     --env=prod --format=dotenv > /tmp/infisical-prod.env
#   scripts/infisical-drift-check.sh /tmp/infisical-prod.env [repo-root]
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

EXPORT_FILE="${1:-}"
ROOT_DIR="${2:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

if [[ -z "$EXPORT_FILE" || ! -f "$EXPORT_FILE" ]]; then
  echo "usage: $0 <infisical-export-dotenv-file> [repo-root]" >&2
  exit 2
fi

GO_CONFIG="$ROOT_DIR/apps/api/internal/platform/config/config.go"
AUTH_ENV="$ROOT_DIR/apps/auth/src/infrastructure/env.ts"
RT_ENV="$ROOT_DIR/apps/realtime/src/infrastructure/env.ts"

for f in "$GO_CONFIG" "$AUTH_ENV" "$RT_ENV"; do
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

export_keys() { grep -oE '^[[:space:]]*[A-Z][A-Z0-9_]*=' "$EXPORT_FILE" | grep -oE '[A-Z][A-Z0-9_]*' | sort -u || true; }

go_keys="$(go_schema "$GO_CONFIG")"
go_req="$(go_required "$GO_CONFIG")"
auth_keys="$(node_schema "$AUTH_ENV")"
auth_req="$(node_required "$AUTH_ENV")"
rt_keys="$(node_schema "$RT_ENV")"
rt_req="$(node_required "$RT_ENV")"
all_app_keys="$(printf '%s\n%s\n%s\n' "$go_keys" "$auth_keys" "$rt_keys" | sort -u)"
all_required="$(printf '%s\n%s\n%s\n' "$go_req" "$auth_req" "$rt_req" | sort -u)"
exported="$(export_keys)"
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

if ((failures > 0)); then
  printf 'infisical-drift-check: %d problem(s) found\n' "$failures" >&2
  exit 1
fi
echo "infisical export matches code schema"
