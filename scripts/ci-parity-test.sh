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
if [[ "${1:-}" == "test" && -n "${CI_PARITY_TEST_SELECTED:-}" ]]; then
  [[ "${API_TEST_DATABASE_REQUIRED:-}" == 1 ]]
  for arg in "${@:2}"; do
    [[ "${arg}" == -* ]] || printf '%s\n' "${arg}" >>"${CI_PARITY_TEST_SELECTED}"
  done
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

# This negative case must not inherit the CI job's real DSN.
if (unset API_TEST_DATABASE_DSN; run_parity api-test) >"${fixture_dir}/missing-dsn.out" 2>&1; then
  echo 'api-test unexpectedly accepted a missing API_TEST_DATABASE_DSN' >&2
  exit 1
fi
if ! grep -Fq 'API_TEST_DATABASE_DSN is required' "${fixture_dir}/missing-dsn.out"; then
  cat "${fixture_dir}/missing-dsn.out" >&2
  exit 1
fi

printf '%s\n' example.test/pkg0 example.test/pkg1 example.test/pkg2 example.test/pkg3 >"${fixture_dir}/expected-packages"
for mode in standard race; do
  : >"${log_file}"
  selected_file="${fixture_dir}/${mode}-packages"
  : >"${selected_file}"
  for shard in 0 1; do
    API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
      API_TEST_DATABASE_REQUIRED=0 \
      TEST_MODE="${mode}" TEST_SHARD="${shard}" TEST_SHARD_COUNT=2 \
      CI_PARITY_TEST_SELECTED="${selected_file}" \
      run_parity api-test >"${fixture_dir}/${mode}-${shard}.out"
    assert_file_contains "${fixture_dir}/${mode}-${shard}.out" \
      "ci-parity: api-test passed mode=${mode} shard=${shard} shard_count=2 elapsed="
  done
  # Compare the actual runner's package arguments without deduplicating them:
  # omitted or repeated packages must both fail this exact-union assertion.
  LC_ALL=C sort "${selected_file}" >"${selected_file}.sorted"
  diff -u "${fixture_dir}/expected-packages" "${selected_file}.sorted"
  [[ "$(grep -c '^go list ./\.\.\.$' "${log_file}")" == 2 ]]
  [[ "$(grep -c '^go test ' "${log_file}")" == 2 ]]
  test_flags='-p=4 -count=1'
  [[ "${mode}" != race ]] || test_flags="-race ${test_flags}"
  assert_contains "go test ${test_flags} example.test/pkg0 example.test/pkg2"
  assert_contains "go test ${test_flags} example.test/pkg1 example.test/pkg3"
done

while IFS='|' read -r shard shard_count expected_error; do
  : >"${log_file}"
  if API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
    TEST_MODE=standard TEST_SHARD="${shard}" TEST_SHARD_COUNT="${shard_count}" \
    run_parity api-test >"${fixture_dir}/invalid-shard.out" 2>&1; then
    echo "api-test unexpectedly accepted shard ${shard}/${shard_count}" >&2
    exit 1
  fi
  assert_file_contains "${fixture_dir}/invalid-shard.out" "${expected_error}"
  if grep -q '^go test ' "${log_file}"; then
    echo 'api-test executed tests for an invalid shard' >&2
    exit 1
  fi
done <<'EOF'
-1|2|TEST_SHARD must be a non-negative integer
invalid|2|TEST_SHARD must be a non-negative integer
0|invalid|TEST_SHARD_COUNT must be a non-negative integer
0|0|TEST_SHARD_COUNT must be greater than zero
2|2|TEST_SHARD must be less than TEST_SHARD_COUNT
4|5|selected no packages
EOF

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
