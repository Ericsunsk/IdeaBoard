#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

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
export DB_HOST="${DB_HOST:-127.0.0.1}"
export DB_PORT="${DB_PORT:-5432}"
export DB_NAME="${DB_NAME:-ideaboard_test}"
export DB_SSLMODE="${DB_SSLMODE:-disable}"
export API_DB_USER="${API_DB_USER:-ideaboard_api}"
export API_DB_PASSWORD="${API_DB_PASSWORD:-ideaboard_api_test}"
export AUTH_DB_USER="${AUTH_DB_USER:-ideaboard_auth}"
export AUTH_DB_PASSWORD="${AUTH_DB_PASSWORD:-ideaboard_auth_test}"
export DB_MIGRATION_USER="${DB_MIGRATION_USER:-ideaboard}"
export DB_MIGRATION_PASSWORD="${DB_MIGRATION_PASSWORD:-ideaboard_test}"
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
  trap 'rm -f -- "${atlas_binary}"' EXIT
  curl -fsSL "https://release.ariga.io/atlas/atlas-linux-${atlas_arch}-v${atlas_version}" -o "${atlas_binary}"
  chmod 0555 "${atlas_binary}"
fi

atlas_url="postgres://${DB_MIGRATION_USER}:${DB_MIGRATION_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"

docker cp \
  "${repo_root}/docker/postgres/init-runtime-roles.sh" \
  "${postgres_container}:/tmp/init-runtime-roles.sh"
docker exec \
  -e POSTGRES_USER="${DB_MIGRATION_USER}" \
  -e POSTGRES_DB="${DB_NAME}" \
  -e API_DB_PASSWORD="${API_DB_PASSWORD}" \
  -e AUTH_DB_PASSWORD="${AUTH_DB_PASSWORD}" \
  "${postgres_container}" \
  sh /tmp/init-runtime-roles.sh

run_api_migration() {
	"${atlas_binary}" migrate apply \
	  --dir "file://${repo_root}/apps/api/internal/db/migrations" \
	  --url "${atlas_url}"
	(cd "${repo_root}/apps/api" && \
	  env DB_USER="${API_DB_USER}" DB_PASSWORD="${API_DB_PASSWORD}" go run ./cmd finalize-migrate)
}

run_auth_migration() {
  (cd "${repo_root}/apps/auth" && \
    env DB_USER="${AUTH_DB_USER}" DB_PASSWORD="${AUTH_DB_PASSWORD}" npm run migrate:dist)
}

run_api_migration
run_auth_migration
run_api_migration
run_auth_migration
