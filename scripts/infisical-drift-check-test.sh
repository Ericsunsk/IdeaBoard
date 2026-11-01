#!/usr/bin/env bash
# infisical-drift-check-test.sh
#
# Self-test for scripts/infisical-drift-check.sh. Builds a minimal fixture
# repo root (code schema) plus synthetic Infisical dotenv exports under a
# temp dir and asserts the checker accepts a complete export while rejecting
# a missing required key, an unknown key, and a deploy-only key stored in
# Infisical. Hermetic: only bash/grep, no network, secrets, or Docker.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

checker="$SCRIPT_DIR/infisical-drift-check.sh"

fixture_root="$(mktemp -d)"
cleanup_path "$fixture_root"

mkdir -p "$fixture_root/apps/api/internal/platform/config"
mkdir -p "$fixture_root/apps/auth/src/infrastructure"
mkdir -p "$fixture_root/apps/realtime/src/infrastructure"
mkdir -p "$fixture_root/apps/ai-proxy/src"
cat >"$fixture_root/apps/ai-proxy/src/contracts.ts" <<'EOF'
export interface Env {
  CF_ACCOUNT_ID: string;
  CF_GATEWAY_ID: string;
  CF_AIG_TOKEN: string;
  WORKER_INTERNAL_SECRET: string;
}
export interface Payload {
  NOT_A_BINDING: string;
}
EOF
cat >"$fixture_root/apps/api/internal/platform/config/config.go" <<'EOF'
package config

type envConfig struct {
	APIPGPassword string `env:"API_PG_PASSWORD"`
	PGDB          string `env:"PG_DB"`
	PGMigrationUser string `env:"PG_MIGRATION_USER"`
	PGMigrationPassword string `env:"PG_MIGRATION_PASSWORD"`
}

func (c *Config) ValidateRequired() error {
	missing = append(missing, "API_PG_PASSWORD")
	return nil
}
EOF
cat >"$fixture_root/apps/auth/src/infrastructure/env.ts" <<'EOF'
export function loadEnv(source: EnvSource) {
  const a = getEnv(source, 'AUTH_INTERNAL_KEY', '')
  const problems: string[] = []
  if (!a) problems.push('AUTH_INTERNAL_KEY is required')
  return { a }
}
EOF
cat >"$fixture_root/apps/realtime/src/infrastructure/env.ts" <<'EOF'
export function loadEnv(source: EnvSource) {
  return {}
}
EOF

expect_pass() {
  if ! bash "$checker" "$1" "$fixture_root" >/dev/null 2>&1; then
    echo "infisical-drift-check-test: expected PASS for $2" >&2
    return 1
  fi
}

expect_fail_with() {
  # NOTE: capture to a variable first — `checker | grep -q` under
  # `set -o pipefail` is racy (grep -q closes the pipe early, checker dies
  # of SIGPIPE with 141 and the pipeline status becomes schedule-dependent).
  local out rc
  set +e
  out="$(bash "$checker" "$1" "$fixture_root" 2>&1)"
  rc=$?
  set -e
  if ((rc == 0)); then
    echo "infisical-drift-check-test: expected FAIL for $2" >&2
    return 1
  fi
  if ! grep -q "$3" <<<"$out"; then
    echo "infisical-drift-check-test: missing marker '$3' for $2" >&2
    return 1
  fi
}

printf 'API_PG_PASSWORD=x\nAUTH_INTERNAL_KEY=x\nPG_DB=x\n' >"$fixture_root/green.env"
printf 'AUTH_INTERNAL_KEY=x\nPG_DB=x\n' >"$fixture_root/missing.env"
printf 'API_PG_PASSWORD=x\nAUTH_INTERNAL_KEY=x\nNOPE_UNKNOWN=x\n' >"$fixture_root/unknown.env"
printf 'API_PG_PASSWORD=x\nAUTH_INTERNAL_KEY=x\nIDEABOARD_API_IMAGE=x\n' >"$fixture_root/deploy.env"

expect_pass "$fixture_root/green.env" "VPS-required export without optional Worker setup"
cp "$fixture_root/green.env" "$fixture_root/worker.env"
printf 'CF_ACCOUNT_ID=x\nCF_GATEWAY_ID=x\nCF_AIG_TOKEN=x\nWORKER_INTERNAL_SECRET=x\n' >>"$fixture_root/worker.env"
expect_pass "$fixture_root/worker.env" "Worker bindings are recognized from their contract"
printf 'NOT_A_BINDING=x\n' >>"$fixture_root/worker.env"
expect_fail_with "$fixture_root/worker.env" "non-Env fields are not bindings" "NOT_A_BINDING"
# Derive new names from the interface rather than a frozen key allowlist.
sed 's/  CF_ACCOUNT_ID: string;/  WORKER_NEW_BINDING: string;/' "$fixture_root/apps/ai-proxy/src/contracts.ts" >"$fixture_root/changed.ts"
mv "$fixture_root/changed.ts" "$fixture_root/apps/ai-proxy/src/contracts.ts"
cp "$fixture_root/green.env" "$fixture_root/worker.env"
printf 'WORKER_NEW_BINDING=x\n' >>"$fixture_root/worker.env"
expect_pass "$fixture_root/worker.env" "new Worker contract binding is recognized"
printf 'CF_ACCOUNT_ID=x\n' >>"$fixture_root/worker.env"
expect_fail_with "$fixture_root/worker.env" "removed Worker binding is rejected" "CF_ACCOUNT_ID"
for owner_key in POSTGRES_PASSWORD PG_MIGRATION_USER PG_MIGRATION_PASSWORD; do
  cp "$fixture_root/green.env" "$fixture_root/owner.env"
  printf '%s=x\n' "$owner_key" >> "$fixture_root/owner.env"
  expect_fail_with "$fixture_root/owner.env" "owner credential in Cloud" "$owner_key must not live in Infisical"
done
expect_fail_with "$fixture_root/missing.env" "missing required" "API_PG_PASSWORD.*missing"
expect_fail_with "$fixture_root/unknown.env" "unknown key" "NOPE_UNKNOWN"
expect_fail_with "$fixture_root/deploy.env" "deploy-only key" "must not live in Infisical"

echo "infisical-drift-check self-test passes"
