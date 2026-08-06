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
  if ! grep -Fq -- "${expected}" "${log_file}"; then
    printf 'Expected command log to contain: %s\n' "${expected}" >&2
    cat "${log_file}" >&2
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
  run_parity api-test
assert_contains 'go list ./...'
assert_contains 'go test -count=1 example.test/pkg1 example.test/pkg3'

: >"${log_file}"
API_TEST_DATABASE_DSN='postgres://test.invalid/ideaboard' \
  TEST_MODE=race TEST_SHARD=0 TEST_SHARD_COUNT=1 \
  run_parity api-test
assert_contains 'go test -race -count=1 example.test/pkg0 example.test/pkg1 example.test/pkg2 example.test/pkg3'

: >"${log_file}"
run_parity web-smoke
assert_contains 'npm run smoke:user'

echo 'ci-parity contract tests passed'
