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
    REALTIME_REQUIRED="${REALTIME_REQUIRED:-false}" REALTIME_RESULT="${REALTIME_RESULT:-skipped}" \
    AI_PROXY_REQUIRED="${AI_PROXY_REQUIRED:-false}" AI_PROXY_RESULT="${AI_PROXY_RESULT:-skipped}" \
    MIGRATION_REQUIRED="${MIGRATION_REQUIRED:-false}" MIGRATION_RESULT="${MIGRATION_RESULT:-skipped}" \
    BROWSER_SMOKE_REQUIRED="${BROWSER_SMOKE_REQUIRED:-false}" BROWSER_SMOKE_RESULT="${BROWSER_SMOKE_RESULT:-skipped}" \
    OPS_REQUIRED="${OPS_REQUIRED:-false}" OPS_RESULT="${OPS_RESULT:-skipped}" \
    MEDIA_CONTRACT_REQUIRED="${MEDIA_CONTRACT_REQUIRED:-true}" MEDIA_CONTRACT_RESULT="${MEDIA_CONTRACT_RESULT:-success}" \
    HANDBOOK_CONTRACT_REQUIRED="${HANDBOOK_CONTRACT_REQUIRED:-true}" HANDBOOK_CONTRACT_RESULT="${HANDBOOK_CONTRACT_RESULT:-success}" \
    PR_SIZE_GUARD_REQUIRED="${PR_SIZE_GUARD_REQUIRED:-true}" PR_SIZE_GUARD_RESULT="${PR_SIZE_GUARD_RESULT:-success}" \
    "${script_dir}/ci-gate.sh"
}

: >"${fixture_dir}/summary.md"
run_gate
grep -Fq '| Web | false | skipped |' "${fixture_dir}/summary.md"
grep -Fq '| AI Proxy | false | skipped |' "${fixture_dir}/summary.md"
grep -Fq '| Backend browser smoke | false | skipped |' "${fixture_dir}/summary.md"
grep -Fq '| Media contract | true | success |' "${fixture_dir}/summary.md"
grep -Fq '| Handbook contract | true | success |' "${fixture_dir}/summary.md"
grep -Fq '| PR size guard | true | success |' "${fixture_dir}/summary.md"

: >"${fixture_dir}/summary.md"
API_REQUIRED=true API_RESULT=success BROWSER_SMOKE_RESULT=success run_gate
grep -Fq '| API | true | success |' "${fixture_dir}/summary.md"

: >"${fixture_dir}/summary.md"
REALTIME_REQUIRED=true REALTIME_RESULT=success BROWSER_SMOKE_RESULT=success run_gate
grep -Fq '| Realtime | true | success |' "${fixture_dir}/summary.md"

if REALTIME_REQUIRED=true REALTIME_RESULT=failure BROWSER_SMOKE_RESULT=success run_gate; then
  echo 'CI gate unexpectedly accepted a failed required Realtime lane' >&2
  exit 1
fi

if API_REQUIRED=true API_RESULT=failure BROWSER_SMOKE_RESULT=success run_gate; then
  echo 'CI gate unexpectedly accepted a failed required API lane' >&2
  exit 1
fi

if API_REQUIRED=true API_RESULT=success BROWSER_SMOKE_REQUIRED=true BROWSER_SMOKE_RESULT=skipped run_gate; then
  echo 'CI gate unexpectedly accepted a missing Backend browser smoke lane' >&2
  exit 1
fi

: >"${fixture_dir}/summary.md"
WEB_REQUIRED=true WEB_RESULT=success API_REQUIRED=true API_RESULT=success BROWSER_SMOKE_REQUIRED=true run_gate

if MEDIA_CONTRACT_REQUIRED=true MEDIA_CONTRACT_RESULT=failure run_gate; then
  echo 'CI gate unexpectedly accepted a failed Media contract lane' >&2
  exit 1
fi

if HANDBOOK_CONTRACT_REQUIRED=true HANDBOOK_CONTRACT_RESULT=failure run_gate; then
  echo 'CI gate unexpectedly accepted a failed Handbook contract lane' >&2
  exit 1
fi

if PR_SIZE_GUARD_REQUIRED=true PR_SIZE_GUARD_RESULT=failure run_gate; then
  echo 'CI gate unexpectedly accepted a failed PR size guard lane' >&2
  exit 1
fi

: >"${fixture_dir}/summary.md"
AI_PROXY_REQUIRED=true AI_PROXY_RESULT=success run_gate
grep -Fq '| AI Proxy | true | success |' "${fixture_dir}/summary.md"
for result in failure cancelled skipped ''; do
  if AI_PROXY_REQUIRED=true AI_PROXY_RESULT="${result}" run_gate; then
    echo "CI gate unexpectedly accepted required AI Proxy result: ${result}" >&2
    exit 1
  fi
done
if CHANGES_RESULT=failure run_gate; then
  echo 'CI gate unexpectedly accepted failed classification' >&2
  exit 1
fi
echo 'CI gate contract tests passed'
