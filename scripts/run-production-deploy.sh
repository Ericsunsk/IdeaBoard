#!/usr/bin/env bash

set -euo pipefail

if (( $# != 1 )); then
  printf 'usage: %s <release-manifest.json>\n' "${0##*/}" >&2
  exit 2
fi

manifest="$1"
[[ -s "${manifest}" ]] || {
  printf 'Production release manifest is missing: %s\n' "${manifest}" >&2
  exit 2
}

: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${VPS_HOST:?VPS_HOST is required}"
: "${VPS_SSH_PORT:?VPS_SSH_PORT is required}"
: "${VPS_DEPLOY_USER:?VPS_DEPLOY_USER is required}"

ssh_key_file="${SSH_KEY_FILE:-${RUNNER_TEMP}/ideaboard-production-ssh/deploy-key}"
ssh_known_hosts_file="${SSH_KNOWN_HOSTS_FILE:-${RUNNER_TEMP}/ideaboard-production-ssh/known_hosts}"
result_file="${PRODUCTION_DEPLOY_RESULT_PATH:-${RUNNER_TEMP}/production-deploy-result.json}"
raw_result="${RUNNER_TEMP}/production-deploy-result.raw"
raw_error="${RUNNER_TEMP}/production-deploy-error.raw"

[[ -r "${ssh_key_file}" ]] || {
  printf 'Production SSH key is missing: %s\n' "${ssh_key_file}" >&2
  exit 2
}
[[ -r "${ssh_known_hosts_file}" ]] || {
  printf 'Production known_hosts file is missing: %s\n' "${ssh_known_hosts_file}" >&2
  exit 2
}

cleanup() {
  rm -f -- "${raw_result}" "${raw_error}"
}
trap cleanup EXIT

ssh_options=(
  -i "${ssh_key_file}"
  -p "${VPS_SSH_PORT}"
  -o BatchMode=yes
  -o IdentitiesOnly=yes
  -o PasswordAuthentication=no
  -o KbdInteractiveAuthentication=no
  -o StrictHostKeyChecking=yes
  -o UserKnownHostsFile="${ssh_known_hosts_file}"
  -o ConnectTimeout=15
  -o ConnectionAttempts=2
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=3
  -o RequestTTY=no
)

is_retryable_stage() {
  case "$1" in
    init|resolve-images|preflight|pull|start-postgres|manifest-validation|lock|checkout|transport)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

run_deploy_attempt() {
  local deploy_status=0
  DEPLOY_TIMED_OUT=0
  timeout --foreground 25m \
    ssh "${ssh_options[@]}" "${VPS_DEPLOY_USER}@${VPS_HOST}" \
    < "${manifest}" \
    > "${raw_result}" 2> "${raw_error}" || deploy_status=$?
  (( deploy_status == 124 )) && DEPLOY_TIMED_OUT=1

  if jq -e -s --argjson deploy_status "${deploy_status}" '
    length == 1
    and (.[0] | type == "object")
    and (.[0].result == "success" or .[0].result == "failure")
    and (.[0].stage | IN(
      "init", "resolve-images", "preflight", "pull", "start-postgres", "backup",
      "api-migration", "auth-migration", "up", "smoke", "complete", "unknown",
      "manifest-validation", "lock", "checkout", "deployment", "transport"
    ))
    and (.[0].recovery_hint | IN(
      "none",
      "inspect_release_state_and_service_logs_on_vps",
      "inspect_release_state_and_backups_before_manual_recovery",
      "inspect_restricted_entrypoint_on_vps",
      "verify_vps_connectivity_and_restricted_deploy_key"
    ))
    and (
      ($deploy_status == 0 and .[0].result == "success" and .[0].stage == "complete" and .[0].recovery_hint == "none")
      or ($deploy_status != 0 and .[0].result == "failure" and .[0].recovery_hint != "none")
    )
  ' "${raw_result}" > /dev/null 2>&1; then
    jq -c '{result, stage, recovery_hint}' "${raw_result}" > "${result_file}"
  else
    printf '%s\n' \
      '{"result":"failure","stage":"transport","recovery_hint":"verify_vps_connectivity_and_restricted_deploy_key"}' \
      > "${result_file}"
    deploy_status=1
  fi

  return "${deploy_status}"
}

deploy_status=0
for attempt in 1 2; do
  deploy_status=0
  run_deploy_attempt || deploy_status=$?
  (( deploy_status == 0 )) && break

  stage="$(jq -r '.stage // "transport"' "${result_file}" 2>/dev/null || printf 'transport')"
  if (( attempt == 1 && DEPLOY_TIMED_OUT == 0 )) && is_retryable_stage "${stage}"; then
    printf 'Retrying Production deployment after retryable stage: %s\n' "${stage}" >&2
    continue
  fi
  break
done

cat "${result_file}"
(( deploy_status == 0 )) || exit "${deploy_status}"
jq -e '.result == "success" and .stage == "complete"' "${result_file}" > /dev/null
