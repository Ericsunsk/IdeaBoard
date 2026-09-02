#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "${fixture_dir}"' EXIT

run_gate() {
  GITHUB_STEP_SUMMARY="${fixture_dir}/summary.md" \
    CHANGES_RESULT="${CHANGES_RESULT:-success}" \
    WEB_REQUIRED="${WEB_REQUIRED:-false}" WEB_RESULT="${WEB_RESULT:-skipped}" \
    API_REQUIRED="${API_REQUIRED:-false}" API_RESULT="${API_RESULT:-skipped}" \
    AUTH_REQUIRED="${AUTH_REQUIRED:-false}" AUTH_RESULT="${AUTH_RESULT:-skipped}" \
    MIGRATION_REQUIRED="${MIGRATION_REQUIRED:-false}" MIGRATION_RESULT="${MIGRATION_RESULT:-skipped}" \
    DOCKER_REQUIRED="${DOCKER_REQUIRED:-false}" BROWSER_SMOKE_RESULT="${BROWSER_SMOKE_RESULT:-skipped}" \
    OPS_REQUIRED="${OPS_REQUIRED:-false}" OPS_RESULT="${OPS_RESULT:-skipped}" \
    MEDIA_CONTRACT_REQUIRED="${MEDIA_CONTRACT_REQUIRED:-true}" MEDIA_CONTRACT_RESULT="${MEDIA_CONTRACT_RESULT:-success}" \
    "${script_dir}/ci-gate.sh"
}

: >"${fixture_dir}/summary.md"
run_gate
grep -Fq '| Web | false | skipped |' "${fixture_dir}/summary.md"
grep -Fq '| Backend browser smoke | derived | skipped |' "${fixture_dir}/summary.md"
grep -Fq '| Media contract | true | success |' "${fixture_dir}/summary.md"

: >"${fixture_dir}/summary.md"
API_REQUIRED=true API_RESULT=success BROWSER_SMOKE_RESULT=success run_gate
grep -Fq '| API | true | success |' "${fixture_dir}/summary.md"

if API_REQUIRED=true API_RESULT=failure BROWSER_SMOKE_RESULT=success run_gate; then
  echo 'CI gate unexpectedly accepted a failed required API lane' >&2
  exit 1
fi

if API_REQUIRED=true API_RESULT=success BROWSER_SMOKE_RESULT=skipped run_gate; then
  echo 'CI gate unexpectedly accepted a missing Backend browser smoke lane' >&2
  exit 1
fi

: >"${fixture_dir}/summary.md"
WEB_REQUIRED=true WEB_RESULT=success API_REQUIRED=true API_RESULT=success run_gate

if MEDIA_CONTRACT_REQUIRED=true MEDIA_CONTRACT_RESULT=failure run_gate; then
  echo 'CI gate unexpectedly accepted a failed Media contract lane' >&2
  exit 1
fi

echo 'CI gate contract tests passed'
