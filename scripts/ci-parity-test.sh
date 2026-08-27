#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "${fixture_dir}"' EXIT

bin_dir="${fixture_dir}/bin"
log_file="${fixture_dir}/commands.log"
mkdir -p "${bin_dir}"

cat >"${bin_dir}/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'go' >>"${CI_PARITY_TEST_LOG}"
printf ' %q' "$@" >>"${CI_PARITY_TEST_LOG}"
printf '\n' >>"${CI_PARITY_TEST_LOG}"
if [[ "${1:-}" == "list" ]]; then
  printf '%s\n' example.test/pkg0 example.test/pkg1 example.test/pkg2 example.test/pkg3
fi
if [[ "${1:-}" == "test" && -n "${CI_PARITY_TEST_GO_EXIT:-}" ]]; then
  exit "${CI_PARITY_TEST_GO_EXIT}"
fi
EOF

cat >"${bin_dir}/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'npm' >>"${CI_PARITY_TEST_LOG}"
printf ' %q' "$@" >>"${CI_PARITY_TEST_LOG}"
printf '\n' >>"${CI_PARITY_TEST_LOG}"
EOF

cat >"${bin_dir}/uname" <<'EOF'
#!/usr/bin/env bash
printf 'Linux\n'
EOF

chmod +x "${bin_dir}/go" "${bin_dir}/npm" "${bin_dir}/uname"

run_parity() {
  PATH="${bin_dir}:${PATH}" \
    CI_PARITY_TEST_LOG="${log_file}" \
    "${repo_root}/scripts/ci-parity.sh" "$@"
}

assert_contains() {
  local expected="$1"
  assert_file_contains "${log_file}" "${expected}"
}

assert_file_contains() {
  local file="$1"
  local expected="$2"
  if ! grep -Fq -- "${expected}" "${file}"; then
    printf 'Expected %s to contain: %s\n' "${file}" "${expected}" >&2
    cat "${file}" >&2
    exit 1
  fi
}

if run_parity api-test >"${fixture_dir}/missing-dsn.out" 2>&1; then
  echo 'api-test unexpectedly accepted a missing API_TEST_DATABASE_DSN' >&2
  exit 1
fi
if ! grep -Fq 'API_TEST_DATABASE_DSN is required' "${fixture_dir}/missing-dsn.out"; then
  cat "${fixture_dir}/missing-dsn.out" >&2
  exit 1
fi

: >"${log_file}"
API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
  TEST_MODE=standard TEST_SHARD=1 TEST_SHARD_COUNT=2 \
  run_parity api-test >"${fixture_dir}/standard.out"
assert_contains 'go list ./...'
assert_contains 'go test -p=4 -count=1 example.test/pkg1 example.test/pkg3'
assert_file_contains "${fixture_dir}/standard.out" 'ci-parity: api-test passed mode=standard shard=1 shard_count=2 elapsed='

: >"${log_file}"
API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
  TEST_MODE=race TEST_SHARD=0 TEST_SHARD_COUNT=1 \
  run_parity api-test
assert_contains 'go test -race -p=4 -count=1 example.test/pkg0 example.test/pkg1 example.test/pkg2 example.test/pkg3'

: >"${log_file}"
run_parity web-smoke
assert_contains 'npm run smoke:user'

if grep -R -n -E 'playwright-cli|install-browser|PLAYWRIGHT_CLI_BROWSER' \
  "${repo_root}/scripts/ci-parity.sh" "${repo_root}/.github/workflows"; then
  echo 'CI browser smoke must use the pinned Playwright image and project Playwright API' >&2
  exit 1
fi

: >"${log_file}"
if API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
  TEST_MODE=standard TEST_SHARD=0 TEST_SHARD_COUNT=1 \
  CI_PARITY_TEST_GO_EXIT=23 \
  run_parity api-test >"${fixture_dir}/failed-api.out" 2>&1; then
  echo 'api-test unexpectedly hid the go test failure' >&2
  exit 1
fi
assert_file_contains "${fixture_dir}/failed-api.out" 'ci-parity: api-test failed mode=standard shard=0 shard_count=1 elapsed='

summary_file="${fixture_dir}/github-step-summary.md"
: >"${summary_file}"
API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
  TEST_MODE=race TEST_SHARD=0 TEST_SHARD_COUNT=1 \
  GITHUB_STEP_SUMMARY="${summary_file}" \
  run_parity api-test >"${fixture_dir}/summary.out"
assert_file_contains "${summary_file}" '### API test'
assert_file_contains "${summary_file}" "- Status: \`passed\`"
assert_file_contains "${summary_file}" "- Mode: \`race\`"
assert_file_contains "${summary_file}" "- Shard: \`0/1\`"
assert_file_contains "${summary_file}" '- Elapsed: `'

echo 'ci-parity contract tests passed'
