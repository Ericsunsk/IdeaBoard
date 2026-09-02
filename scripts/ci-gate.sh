#!/usr/bin/env bash

set -euo pipefail

summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

lanes=(
  'Web|WEB_REQUIRED|WEB_RESULT'
  'API|API_REQUIRED|API_RESULT'
  'Auth|AUTH_REQUIRED|AUTH_RESULT'
  'Migration integration|MIGRATION_REQUIRED|MIGRATION_RESULT'
  'Ops|OPS_REQUIRED|OPS_RESULT'
  'Media contract|MEDIA_CONTRACT_REQUIRED|MEDIA_CONTRACT_RESULT'
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
  echo "| Backend browser smoke | derived | ${BROWSER_SMOKE_RESULT:-} |"
  echo
  echo 'Job durations remain available in the native GitHub Actions timing view.'
} >>"${summary_file}"

[[ "${CHANGES_RESULT:-}" == success ]] || exit 1
for lane in "${lanes[@]}"; do
  IFS='|' read -r _ required_name result_name <<<"${lane}"
  [[ "${!required_name:-}" != true || "${!result_name:-}" == success ]] || exit 1
done
if [[ "${API_REQUIRED:-}" == true || "${AUTH_REQUIRED:-}" == true || "${DOCKER_REQUIRED:-}" == true ]]; then
  [[ "${WEB_REQUIRED:-}" == true || "${BROWSER_SMOKE_RESULT:-}" == success ]] || exit 1
fi
