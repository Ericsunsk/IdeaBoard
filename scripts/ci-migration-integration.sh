#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via script_dir; scripts/lib/common.sh is linted directly in ops.yml.
source "${script_dir}/lib/common.sh"

postgres_image="${CI_MIGRATION_POSTGRES_IMAGE:-postgres:17-alpine}"
postgres_container="${CI_MIGRATION_POSTGRES_CONTAINER:-}"

if [[ -z "${postgres_container}" ]]; then
  postgres_container="$(docker ps --filter "ancestor=${postgres_image}" --format '{{.ID}}' | head -n 1)"
fi
if [[ -z "${postgres_container}" ]]; then
  printf 'ci-migration-integration: no running %s container found\n' "${postgres_image}" >&2
  exit 2
fi

export APP_ENV="${APP_ENV:-production}"
export PG_HOST="${PG_HOST:-127.0.0.1}"
export PG_PORT="${PG_PORT:-5432}"
export PG_DB="${PG_DB:-ideaboard_test}"
export PG_SSLMODE="${PG_SSLMODE:-disable}"
export API_PG_USER="${API_PG_USER:-ideaboard_api}"
export API_PG_PASSWORD="${API_PG_PASSWORD:-ideaboard_api_test}"
export AUTH_PG_USER="${AUTH_PG_USER:-ideaboard_auth}"
export AUTH_PG_PASSWORD="${AUTH_PG_PASSWORD:-ideaboard_auth_test}"
export PG_MIGRATION_USER="${PG_MIGRATION_USER:-ideaboard}"
export PG_MIGRATION_PASSWORD="${PG_MIGRATION_PASSWORD:-ideaboard_test}"
export AUTH_INTERNAL_KEY="${AUTH_INTERNAL_KEY:-0123456789abcdef0123456789abcdef}"
export APP_SECRETS_ENCRYPTION_KEY="${APP_SECRETS_ENCRYPTION_KEY:-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=}"
export PUBLIC_ORIGIN="${PUBLIC_ORIGIN:-https://app.example.com}"
export BETTER_AUTH_SECRET="${BETTER_AUTH_SECRET:-abcdef0123456789abcdef0123456789}"
export NODE_ENV="${NODE_ENV:-production}"

(cd "${repo_root}/apps/auth" && npm run build)

atlas_version="${ATLAS_VERSION:-1.3.2}"
atlas_arch="$(uname -m)"
case "${atlas_arch}" in
  x86_64) atlas_arch=amd64 ;;
  aarch64|arm64) atlas_arch=arm64 ;;
  *) printf 'ci-migration-integration: unsupported host architecture %s\n' "${atlas_arch}" >&2; exit 2 ;;
esac
atlas_binary="${CI_MIGRATION_ATLAS_BINARY:-}"
if [[ -z "${atlas_binary}" ]]; then
  atlas_binary="$(mktemp)"
  cleanup_path "${atlas_binary}"
  curl -fsSL "https://release.ariga.io/atlas/atlas-linux-${atlas_arch}-v${atlas_version}" -o "${atlas_binary}"
  chmod 0555 "${atlas_binary}"
fi

atlas_url="postgres://${PG_MIGRATION_USER}:${PG_MIGRATION_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DB}?sslmode=${PG_SSLMODE}"

docker cp \
  "${repo_root}/docker/postgres/init-runtime-roles.sh" \
  "${postgres_container}:/tmp/init-runtime-roles.sh"
docker exec \
  -e POSTGRES_USER="${PG_MIGRATION_USER}" \
  -e POSTGRES_DB="${PG_DB}" \
  -e API_PG_PASSWORD="${API_PG_PASSWORD}" \
  -e AUTH_PG_PASSWORD="${AUTH_PG_PASSWORD}" \
  "${postgres_container}" \
  sh /tmp/init-runtime-roles.sh

run_api_migration() {
	"${atlas_binary}" migrate apply \
	  --dir "file://${repo_root}/apps/api/internal/db/migrations" \
	  --url "${atlas_url}"
	(cd "${repo_root}/apps/api" && \
	  env API_PG_USER="${API_PG_USER}" API_PG_PASSWORD="${API_PG_PASSWORD}" go run ./cmd finalize-migrate)
}

run_auth_migration() {
  (cd "${repo_root}/apps/auth" && \
    env AUTH_PG_USER="${AUTH_PG_USER}" AUTH_PG_PASSWORD="${AUTH_PG_PASSWORD}" npm run migrate:dist)
}

run_api_migration
run_auth_migration
run_api_migration
run_auth_migration
