#!/usr/bin/env bash

set -euo pipefail

summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

lanes=(
  'Web|WEB_REQUIRED|WEB_RESULT'
  'API|API_REQUIRED|API_RESULT'
  'Auth|AUTH_REQUIRED|AUTH_RESULT'
  'Realtime|REALTIME_REQUIRED|REALTIME_RESULT'
  'Migration integration|MIGRATION_REQUIRED|MIGRATION_RESULT'
  'Ops|OPS_REQUIRED|OPS_RESULT'
  'Media contract|MEDIA_CONTRACT_REQUIRED|MEDIA_CONTRACT_RESULT'
  'Handbook contract|HANDBOOK_CONTRACT_REQUIRED|HANDBOOK_CONTRACT_RESULT'
  'PR size guard|PR_SIZE_GUARD_REQUIRED|PR_SIZE_GUARD_RESULT'
  'Backend browser smoke|BROWSER_SMOKE_REQUIRED|BROWSER_SMOKE_RESULT'
)

{
  echo '## CI verification lanes'
  echo
  echo '| Lane | Required | Result |'
  echo '| --- | --- | --- |'
  echo "| Change classification | yes | ${CHANGES_RESULT:-} |"
  for lane in "${lanes[@]}"; do
    IFS='|' read -r label required_name result_name <<<"${lane}"
    echo "| ${label} | ${!required_name:-false} | ${!result_name:-} |"
  done
  echo
  echo 'Job durations remain available in the native GitHub Actions timing view.'
} >>"${summary_file}"

[[ "${CHANGES_RESULT:-}" == success ]] || exit 1
for lane in "${lanes[@]}"; do
  IFS='|' read -r _ required_name result_name <<<"${lane}"
  if [[ "${required_name}" == BROWSER_SMOKE_REQUIRED && "${WEB_REQUIRED:-}" == true ]]; then
    continue # Web Verification owns the same smoke check.
  fi
  [[ "${!required_name:-}" != true || "${!result_name:-}" == success ]] || exit 1
done
