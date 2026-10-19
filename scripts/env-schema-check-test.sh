#!/usr/bin/env bash
# env-schema-check-test.sh
#
# Self-test for scripts/env-schema-check.sh. Builds minimal fixture repo
# roots under a temp dir and asserts the gate accepts an in-sync tree while
# rejecting each drift class (missing required, unknown example key, legacy
# name, weak example value, unbacked compose env). Requires only bash/grep.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

checker="$SCRIPT_DIR/env-schema-check.sh"

fixture_root="$(mktemp -d)"
cleanup_path "$fixture_root"

write_tree() {
  local root="$1"
  mkdir -p "$root/apps/api/internal/platform/config"
  mkdir -p "$root/apps/auth/src/infrastructure"
  mkdir -p "$root/apps/realtime/src/infrastructure"
  mkdir -p "$root/docker/postgres"
  cat >"$root/apps/api/internal/platform/config/config.go" <<'EOF'
package config

type envConfig struct {
	PGHost              string `env:"PG_HOST"`
	PGPort              string `env:"PG_PORT"`
	PGDB                string `env:"PG_DB"`
	PGSSLMode           string `env:"PG_SSLMODE"`
	APIPGPassword       string `env:"API_PG_PASSWORD"`
	PGMigrationUser     string `env:"PG_MIGRATION_USER"`
	PGMigrationPassword string `env:"PG_MIGRATION_PASSWORD"`
}

func (c *Config) ValidateRequired() error {
	missing = append(missing, "API_PG_PASSWORD")
	return nil
}
EOF
  cat >"$root/apps/auth/src/infrastructure/env.ts" <<'EOF'
export function loadEnv(source: EnvSource) {
  const a = getEnv(source, 'AUTH_INTERNAL_KEY', '')
  const b = getEnvInt(source, 'AUTH_PORT', 6001)
  const problems: string[] = []
  if (!a) problems.push('AUTH_INTERNAL_KEY is required')
  return { a, b }
}
EOF
  cat >"$root/apps/realtime/src/infrastructure/env.ts" <<'EOF'
export function loadEnv(source: EnvSource) {
  const a = getEnv(source, 'RT_TICKET_SECRET', '')
  const problems: string[] = []
  if (!a) problems.push('RT_TICKET_SECRET is required')
  return { a }
}
EOF
  cat >"$root/apps/api/.env.example" <<'EOF'
PG_HOST=127.0.0.1
API_PG_PASSWORD=
EOF
  cat >"$root/apps/auth/.env.example" <<'EOF'
AUTH_INTERNAL_KEY=
# AUTH_PORT=6001
EOF
  cat >"$root/apps/realtime/.env.example" <<'EOF'
RT_TICKET_SECRET=replace-with-ticket-secret
EOF
  cat >"$root/docker/.env.example" <<'EOF'
API_PG_PASSWORD=replace-with-api-password
EOF
  cat >"$root/docker/images.env.example" <<'EOF'
IDEABOARD_API_IMAGE=ghcr.io/o/r/ideaboard-api@sha256:replace-with-api-image-digest
EOF
  cat >"$root/docker/.env" <<'EOF'
DOTENV_PUBLIC_KEY="022feec2b40ebda8983df6326a4351d2746cb834456f96b773d60928cad7136bf8"
API_PG_PASSWORD=encrypted:placeholder-ciphertext-for-gate-fixture
EOF
  cat >"$root/docker/migration.env.example" <<'EOF'
PG_HOST=127.0.0.1
PG_PORT=5432
PG_DB=ideaboard
PG_SSLMODE=disable
PG_MIGRATION_USER=ideaboard
PG_MIGRATION_PASSWORD=replace-with-owner-db-password
EOF
  cat >"$root/docker/compose.yml" <<'EOF'
services:
  api:
    image: ${IDEABOARD_API_IMAGE:?Set image}
    environment:
      PG_HOST: pghost
      API_PG_PASSWORD: ${API_PG_PASSWORD:?Set password}
EOF
  cat >"$root/docker/compose.local.yml" <<'EOF'
services:
  postgres:
    environment:
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?Set password}
EOF
  cat >"$root/docker/postgres/init-runtime-roles.sh" <<'EOF'
#!/bin/sh
: "${API_PG_PASSWORD:?required}"
EOF
  # Negative controls for check C: legitimate longer names that merely
  # contain a legacy-looking substring must never be flagged. They ship in
  # every fixture so each expect_pass also proves no substring false
  # positive (VITE_API_BASE_URL and QA_API_BASE_URL mirror real tracked
  # files; DB_HOSTNAME/MY_DB_HOST/REALTIME_HOSTNAME pin the word boundary).
  mkdir -p "$root/apps/web"
  cat >"$root/apps/web/.env.example" <<'EOF'
VITE_API_BASE_URL=
EOF
  mkdir -p "$root/scripts"
  cat >"$root/scripts/qa-cleanup-smoke.sh" <<'EOF'
#!/usr/bin/env bash
api_base_url="${QA_API_BASE_URL:-${PUBLIC_ORIGIN:?Set PUBLIC_ORIGIN or QA_API_BASE_URL}}"
db_host_name="${DB_HOSTNAME:-localhost}"
my_db_host="${MY_DB_HOST:-localhost}"
realtime_host_name="${REALTIME_HOSTNAME:-localhost}"
EOF
}

failures=0
expect_pass() {
  local name="$1"
  local root="$2"
  if "$checker" "$root" >/dev/null 2>&1; then
    echo "ok: $name"
  else
    echo "FAIL (expected pass): $name" >&2
    failures=$((failures + 1))
  fi
}
expect_fail() {
  local name="$1"
  local root="$2"
  local message="$3"
  local output
  if output="$("$checker" "$root" 2>&1)"; then
    echo "FAIL (expected rejection): $name" >&2
    failures=$((failures + 1))
  elif grep -Fq "$message" <<<"$output"; then
    echo "ok: $name"
  else
    echo "FAIL (wrong error for $name): $output" >&2
    failures=$((failures + 1))
  fi
}

valid="$fixture_root/valid"
write_tree "$valid"
expect_pass "in-sync tree passes" "$valid"

missing_required="$fixture_root/missing-required"
write_tree "$missing_required"
printf 'PG_HOST=127.0.0.1\n' >"$missing_required/apps/api/.env.example"
expect_fail "missing required key is rejected" "$missing_required" "A: required API_PG_PASSWORD"

unknown_key="$fixture_root/unknown-key"
write_tree "$unknown_key"
printf 'PG_HOST=127.0.0.1\nAPI_PG_PASSWORD=\nSTALE_KEY=value\n' >"$unknown_key/apps/api/.env.example"
expect_fail "unknown example key is rejected" "$unknown_key" "B:"

legacy="$fixture_root/legacy"
write_tree "$legacy"
printf 'PG_HOST=127.0.0.1\nAPI_PG_PASSWORD=\n# renamed from DB_PASSWORD\nDB_PASSWORD=old\n' >"$legacy/apps/api/.env.example"
expect_fail "legacy name is rejected" "$legacy" "C:"

# The frozen ADR-0019 body keeps the pre-rename REALTIME_TICKET_SECRET as
# history (rename recorded in its Status appendix); the C gate exempts only
# that file, every other legacy reference still fails.
frozen_adr="$fixture_root/frozen-adr"
write_tree "$frozen_adr"
mkdir -p "$frozen_adr/docs/adr"
printf 'frozen body cites REALTIME_TICKET_SECRET as history\n' >"$frozen_adr/docs/adr/0019-realtime-presence.md"
expect_pass "frozen ADR-0019 legacy reference passes" "$frozen_adr"

other_legacy="$fixture_root/other-legacy"
write_tree "$other_legacy"
mkdir -p "$other_legacy/docs/adr"
printf 'new doc cites REALTIME_TICKET_SECRET as current\n' >"$other_legacy/docs/adr/0022-some-new-doc.md"
expect_fail "legacy name outside frozen ADR-0019 is rejected" "$other_legacy" "C:"

# Dockerfile carrying a removed legacy name (the real-world miss): the
# recursive fallback scan covers files outside the known doc list.
docker_legacy="$fixture_root/docker-legacy"
write_tree "$docker_legacy"
cat >"$docker_legacy/apps/realtime/Dockerfile" <<'EOF'
FROM node:22-alpine
ENV NODE_ENV=production \
    REALTIME_HOST=0.0.0.0 \
    REALTIME_PORT=6002
EOF
expect_fail "legacy name in Dockerfile is rejected" "$docker_legacy" "C: legacy env name still referenced: REALTIME_HOST"

# The git-grep branch (tracked files) had its own no-op bug, so it gets its
# own red/green pair in a real git repo. git is guaranteed in the ops job;
# skip gracefully elsewhere.
if command -v git >/dev/null 2>&1; then
  git_legacy="$fixture_root/git-legacy"
  write_tree "$git_legacy"
  cat >"$git_legacy/apps/realtime/Dockerfile" <<'EOF'
FROM node:22-alpine
ENV NODE_ENV=production \
    REALTIME_HOST=0.0.0.0 \
    REALTIME_PORT=6002
EOF
  git -C "$git_legacy" -c init.defaultBranch=main init -q
  git -C "$git_legacy" add -A
  expect_fail "legacy name in tracked Dockerfile is rejected (git path)" "$git_legacy" "C: legacy env name still referenced: REALTIME_HOST"

  git_clean="$fixture_root/git-clean"
  write_tree "$git_clean"
  cat >"$git_clean/apps/realtime/Dockerfile" <<'EOF'
FROM node:22-alpine
ENV NODE_ENV=production \
    RT_HOST=0.0.0.0 \
    RT_PORT=6002
EOF
  git -C "$git_clean" -c init.defaultBranch=main init -q
  git -C "$git_clean" add -A
  expect_pass "clean tracked tree passes (git path)" "$git_clean"
else
  echo "skip: git not found, git-path legacy cases skipped" >&2
fi

weak="$fixture_root/weak"
write_tree "$weak"
printf 'PG_HOST=127.0.0.1\nAPI_PG_PASSWORD=ideaboard123\n' >"$weak/apps/api/.env.example"
expect_fail "weak example value is rejected" "$weak" "D:"

unbacked="$fixture_root/unbacked"
write_tree "$unbacked"
cat >"$unbacked/docker/compose.yml" <<'EOF'
services:
  api:
    image: ${IDEABOARD_API_IMAGE:?Set image}
    environment:
      PG_HOST: pghost
      API_PG_PASSWORD: ${API_PG_PASSWORD:?Set password}
      MADE_UP_VAR: lit
EOF
expect_fail "unbacked compose env is rejected" "$unbacked" "E:"

private_key="$fixture_root/private-key"
write_tree "$private_key"
printf 'DOTENV_PRIVATE_KEY=should-never-be-committed\n' >>"$private_key/docker/.env"
expect_fail "private key in encrypted env is rejected" "$private_key" "F:"

unknown_encrypted_key="$fixture_root/unknown-encrypted-key"
write_tree "$unknown_encrypted_key"
printf 'MADE_UP_KEY=encrypted:placeholder-ciphertext\n' >>"$unknown_encrypted_key/docker/.env"
expect_fail "unknown key in encrypted env is rejected" "$unknown_encrypted_key" "F:"

images_in_secrets="$fixture_root/images-in-secrets"
write_tree "$images_in_secrets"
printf 'IDEABOARD_API_IMAGE=encrypted:old-ref\n' >> "$images_in_secrets/docker/.env"
expect_fail "image refs cannot return to frozen secrets" "$images_in_secrets" "must not store image refs"

plaintext_in_secrets="$fixture_root/plaintext-in-secrets"
write_tree "$plaintext_in_secrets"
printf 'RIVER_WORKER_CONCURRENCY=encrypted:old-ciphertext\n' >> "$plaintext_in_secrets/docker/.env"
expect_fail "plaintext concurrency cannot return to frozen secrets" "$plaintext_in_secrets" "must not store plaintext RIVER_WORKER_CONCURRENCY"

if (( failures > 0 )); then
  echo "env-schema-check contract tests failed: $failures" >&2
  exit 1
fi
echo "env-schema-check contract tests passed"
