#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
playwright_image="${CI_PARITY_PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v1.61.1-noble}"

usage() {
  cat <<'EOF'
Usage: scripts/ci-parity.sh [all|api-test|web-smoke]

Commands:
  all        Run unsharded standard API tests, then Web smoke in Linux geometry.
  api-test   Run the API test shard selected by TEST_MODE, TEST_SHARD, and
             TEST_SHARD_COUNT. API_TEST_DATABASE_DSN is required.
  web-smoke  Run User route smoke directly on Linux or in a Playwright Linux
             container on other hosts.
EOF
}

fail() {
  printf 'ci-parity: %s\n' "$1" >&2
  exit 2
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_non_negative_integer() {
  local name="$1"
  local value="$2"
  [[ "${value}" =~ ^[0-9]+$ ]] || fail "${name} must be a non-negative integer"
}

record_api_test_timing() {
  local mode="$1"
  local shard="$2"
  local shard_count="$3"
  local elapsed_seconds="$4"
  local status="$5"

  printf 'ci-parity: api-test %s mode=%s shard=%s shard_count=%s elapsed=%ss\n' \
    "${status}" "${mode}" "${shard}" "${shard_count}" "${elapsed_seconds}"

  [[ -n "${GITHUB_STEP_SUMMARY:-}" ]] || return 0

  if ! {
    printf '### API test\n\n'
    printf -- "- Status: \`%s\`\n" "${status}"
    printf -- "- Mode: \`%s\`\n" "${mode}"
    printf -- "- Shard: \`%s/%s\`\n" "${shard}" "${shard_count}"
    printf -- "- Elapsed: \`%ss\`\n" "${elapsed_seconds}"
  } >>"${GITHUB_STEP_SUMMARY}"; then
    printf 'ci-parity: unable to append API timing to GITHUB_STEP_SUMMARY\n' >&2
  fi
}

run_api_tests() {
  local mode="${TEST_MODE:-standard}"
  local shard="${TEST_SHARD:-0}"
  local shard_count="${TEST_SHARD_COUNT:-1}"
  local package
  local started_at
  local elapsed_seconds
  local test_exit_code=0
  local -a packages=()

  [[ -n "${API_TEST_DATABASE_DSN:-}" ]] || \
    fail 'API_TEST_DATABASE_DSN is required for parity API tests; integration tests must not be skipped'
  [[ "${mode}" == "standard" || "${mode}" == "race" ]] || \
    fail 'TEST_MODE must be standard or race'
  require_non_negative_integer TEST_SHARD "${shard}"
  require_non_negative_integer TEST_SHARD_COUNT "${shard_count}"
  (( shard_count > 0 )) || fail 'TEST_SHARD_COUNT must be greater than zero'
  (( shard < shard_count )) || fail 'TEST_SHARD must be less than TEST_SHARD_COUNT'
  require_command go
  started_at="${SECONDS}"

  while IFS= read -r package; do
    [[ -n "${package}" ]] && packages+=("${package}")
  done < <(
    cd "${repo_root}/apps/api"
    go list ./... | awk \
      -v shard="${shard}" \
      -v total="${shard_count}" \
      '((NR - 1) % total) == shard'
  )

  ((${#packages[@]} > 0)) || fail "API shard ${shard}/${shard_count} selected no packages"
  # Snapshot-backed fixtures initialize quickly enough to exhaust PostgreSQL's
  # lock table when Go starts every package at once. Keep package concurrency
  # bounded at the test-runner boundary; each CI shard still runs independently.
  printf 'ci-parity: running API tests mode=%s shard=%s/%s packages=%s package_parallelism=4\n' \
    "${mode}" "${shard}" "${shard_count}" "${#packages[@]}"

  export API_TEST_DATABASE_REQUIRED=1
  if [[ "${mode}" == "race" ]]; then
    if (cd "${repo_root}/apps/api" && go test -race -p=4 -count=1 "${packages[@]}"); then
      :
    else
      test_exit_code=$?
    fi
  else
    if (cd "${repo_root}/apps/api" && go test -p=4 -count=1 "${packages[@]}"); then
      :
    else
      test_exit_code=$?
    fi
  fi

  elapsed_seconds="$((SECONDS - started_at))"
  if (( test_exit_code == 0 )); then
    record_api_test_timing "${mode}" "${shard}" "${shard_count}" "${elapsed_seconds}" passed
  else
    record_api_test_timing "${mode}" "${shard}" "${shard_count}" "${elapsed_seconds}" failed
  fi

  return "${test_exit_code}"
}

run_web_smoke_on_host() {
  require_command npm
  (cd "${repo_root}/apps/web" && npm run smoke:user)
}

run_web_smoke() {
  if [[ "$(uname -s)" == "Linux" ]]; then
    run_web_smoke_on_host
    return
  fi

  require_command docker
  docker info >/dev/null 2>&1 || fail 'Docker is required and must be running for Linux-equivalent Web smoke'
  printf 'Running Web smoke in Linux container %s\n' "${playwright_image}"
  docker run --rm --init \
    --volume "${repo_root}:/workspace:ro" \
    --env USER_ROUTE_SMOKE_VERBOSE="${USER_ROUTE_SMOKE_VERBOSE:-1}" \
    "${playwright_image}" \
    bash -euo pipefail -c '
      cp -a /workspace/apps/web /tmp/ideaboard-web
      cd /tmp/ideaboard-web
      npm ci --no-audit --fund=false
      npm run smoke:user
    '
}

command_name="${1:-all}"
case "${command_name}" in
  all)
    TEST_MODE=standard TEST_SHARD=0 TEST_SHARD_COUNT=1 run_api_tests
    run_web_smoke
    ;;
  api-test)
    run_api_tests
    ;;
  web-smoke)
    run_web_smoke
    ;;
  help | -h | --help)
    usage
    ;;
  *)
    usage >&2
    fail "unknown command: ${command_name}"
    ;;
esac
