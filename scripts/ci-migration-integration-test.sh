#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "${fixture_dir}"' EXIT

bin_dir="${fixture_dir}/bin"
log_file="${fixture_dir}/commands.log"
mkdir -p "${bin_dir}"

for command_name in docker go npm atlas; do
  command_path="${bin_dir}/${command_name}"
  sed "s/__COMMAND__/${command_name}/g" >"${command_path}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '__COMMAND__' >>"${CI_MIGRATION_TEST_LOG}"
if [[ -n "${API_PG_USER:-}" ]]; then
  printf ' API_PG_USER=%q API_PG_PASSWORD=%q' "${API_PG_USER}" "${API_PG_PASSWORD:-}" >>"${CI_MIGRATION_TEST_LOG}"
fi
if [[ -n "${AUTH_PG_USER:-}" ]]; then
  printf ' AUTH_PG_USER=%q AUTH_PG_PASSWORD=%q' "${AUTH_PG_USER}" "${AUTH_PG_PASSWORD:-}" >>"${CI_MIGRATION_TEST_LOG}"
fi
printf ' %q' "$@" >>"${CI_MIGRATION_TEST_LOG}"
printf '\n' >>"${CI_MIGRATION_TEST_LOG}"
EOF
  chmod +x "${command_path}"
done

atlas_binary="${bin_dir}/atlas"

PATH="${bin_dir}:${PATH}" \
  CI_MIGRATION_TEST_LOG="${log_file}" \
  CI_MIGRATION_ATLAS_BINARY="${atlas_binary}" \
  CI_MIGRATION_POSTGRES_CONTAINER="test-postgres" \
  "${repo_root}/scripts/ci-migration-integration.sh"

expected_order="${fixture_dir}/expected.log"
cat >"${expected_order}" <<EOF
npm API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test run build
docker API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test cp ${repo_root}/docker/postgres/init-runtime-roles.sh test-postgres:/tmp/init-runtime-roles.sh
docker API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test exec -e POSTGRES_USER=ideaboard -e POSTGRES_DB=ideaboard_test -e API_PG_PASSWORD=ideaboard_api_test -e AUTH_PG_PASSWORD=ideaboard_auth_test test-postgres sh /tmp/init-runtime-roles.sh
atlas API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test migrate apply --dir file://${repo_root}/apps/api/internal/db/migrations --url postgres://ideaboard:ideaboard_test@127.0.0.1:5432/ideaboard_test\\?sslmode=disable
go API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test run ./cmd finalize-migrate
npm API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test run migrate:dist
atlas API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test migrate apply --dir file://${repo_root}/apps/api/internal/db/migrations --url postgres://ideaboard:ideaboard_test@127.0.0.1:5432/ideaboard_test\\?sslmode=disable
go API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test run ./cmd finalize-migrate
npm API_PG_USER=ideaboard_api API_PG_PASSWORD=ideaboard_api_test AUTH_PG_USER=ideaboard_auth AUTH_PG_PASSWORD=ideaboard_auth_test run migrate:dist
EOF

normalized_log="${fixture_dir}/normalized.log"
sed -E 's/([^ ]+)\/bin\/(go|npm|atlas)/\2/g' "${log_file}" >"${normalized_log}"
diff -u "${expected_order}" "${normalized_log}"

if PATH="${bin_dir}:${PATH}" \
  CI_MIGRATION_TEST_LOG="${log_file}" \
  CI_MIGRATION_ATLAS_BINARY="${atlas_binary}" \
  CI_MIGRATION_POSTGRES_IMAGE="missing-postgres" \
  "${repo_root}/scripts/ci-migration-integration.sh" >"${fixture_dir}/missing.out" 2>&1; then
  echo 'migration integration unexpectedly accepted a missing PostgreSQL container' >&2
  exit 1
fi
grep -Fq 'no running missing-postgres container found' "${fixture_dir}/missing.out"

echo 'CI migration integration contract tests passed'
