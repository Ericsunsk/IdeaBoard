#!/usr/bin/env bash

set -euo pipefail
umask 077

STACK_DIR="${IDEABOARD_PRODUCTION_STACK_DIR:-/opt/stacks/ideaboard}"
LOCK_FILE="${IDEABOARD_PRODUCTION_LOCK_FILE:-/run/lock/ideaboard-production-deploy.lock}"
TMP_DIR="${IDEABOARD_PRODUCTION_TMP_DIR:-/var/tmp}"
EXPECTED_REPOSITORY="${IDEABOARD_PRODUCTION_EXPECTED_REPOSITORY:-Ericsunsk/IdeaBoard}"
MAX_MANIFEST_BYTES=65536
MANIFEST_FILE=""
DEPLOY_RESULT_FILE=""
ENTRYPOINT_STAGE="manifest-validation"

die() {
  printf '{"result":"failure","stage":"%s","recovery_hint":"inspect_restricted_entrypoint_on_vps"}\n' \
    "${ENTRYPOINT_STAGE}"
  printf 'Production deployment rejected: %s\n' "$*" >&2
  exit 1
}

# shellcheck disable=SC2317,SC2329 # Invoked indirectly by the EXIT trap below.
cleanup() {
  if [[ -n "${MANIFEST_FILE}" ]]; then
    rm -f -- "${MANIFEST_FILE}"
  fi
  if [[ -n "${DEPLOY_RESULT_FILE}" ]]; then
    rm -f -- "${DEPLOY_RESULT_FILE}"
  fi
}
trap cleanup EXIT

validate_image_ref() {
  local value="$1"
  local component="$2"
  local expected_image_repository prefix digest

  expected_image_repository="$(printf '%s' "${EXPECTED_REPOSITORY}" | tr '[:upper:]' '[:lower:]')"
  prefix="ghcr.io/${expected_image_repository}/ideaboard-${component}@sha256:"
  [[ "${value}" == "${prefix}"* ]] || return 1
  digest="${value#"${prefix}"}"
  [[ "${digest}" =~ ^[0-9a-f]{64}$ ]]
}

(( $# == 0 )) || die 'command arguments are not accepted'
[[ -z "${SSH_ORIGINAL_COMMAND:-}" ]] || die 'SSH commands are not accepted'

for required_command in git head jq flock mktemp; do
  command -v "${required_command}" >/dev/null 2>&1 || die "missing command: ${required_command}"
done

[[ -d "${STACK_DIR}/.git" ]] || die "Production checkout is unavailable"
[[ -d "${TMP_DIR}" ]] || die "temporary directory is unavailable"

MANIFEST_FILE="$(mktemp "${TMP_DIR%/}/ideaboard-production-manifest.XXXXXX")"
chmod 600 "${MANIFEST_FILE}"
head -c "$((MAX_MANIFEST_BYTES + 1))" > "${MANIFEST_FILE}"
manifest_size="$(wc -c < "${MANIFEST_FILE}" | tr -d '[:space:]')"
(( manifest_size > 0 )) || die 'manifest input is empty'
(( manifest_size <= MAX_MANIFEST_BYTES )) || die 'manifest input exceeds 65536 bytes'

if ! jq -e -s 'length == 1 and (.[0] | type == "object")' \
  "${MANIFEST_FILE}" >/dev/null 2>&1; then
  die 'manifest must contain exactly one JSON object'
fi

tag="$(jq -er '.tag | strings' "${MANIFEST_FILE}")" || die 'manifest tag is missing'
commit="$(jq -er '.commit | strings' "${MANIFEST_FILE}")" || die 'manifest commit is missing'
repository="$(jq -er '.repository | strings' "${MANIFEST_FILE}")" || die 'manifest repository is missing'
api_image="$(jq -er '.images.api | strings' "${MANIFEST_FILE}")" || die 'manifest API image is missing'
api_migrate_image="$(jq -er '.images["api-migrate"] | strings' "${MANIFEST_FILE}")" || \
  die 'manifest API Migration image is missing'
auth_image="$(jq -er '.images.auth | strings' "${MANIFEST_FILE}")" || die 'manifest Auth image is missing'
auth_migrate_image="$(jq -er '.images["auth-migrate"] | strings' "${MANIFEST_FILE}")" || \
  die 'manifest Auth Migration image is missing'
[[ "${tag}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || \
  die 'manifest tag must match vMAJOR.MINOR.PATCH'
[[ "${commit}" =~ ^[0-9a-f]{40}$ ]] || die 'manifest commit must be a full Git SHA'
[[ "${repository}" == "${EXPECTED_REPOSITORY}" ]] || die 'manifest repository is not allowed'
validate_image_ref "${api_image}" api || die 'manifest API image must use the expected immutable digest'
validate_image_ref "${api_migrate_image}" api-migrate || \
  die 'manifest API Migration image must use the expected immutable digest'
validate_image_ref "${auth_image}" auth || die 'manifest Auth image must use the expected immutable digest'
validate_image_ref "${auth_migrate_image}" auth-migrate || \
  die 'manifest Auth Migration image must use the expected immutable digest'

ENTRYPOINT_STAGE="lock"
mkdir -p "$(dirname "${LOCK_FILE}")"
exec 9>"${LOCK_FILE}"
flock -n 9 || die 'another Production deployment is running'

ENTRYPOINT_STAGE="checkout"
if [[ -n "$(git -C "${STACK_DIR}" status --porcelain --untracked-files=no)" ]]; then
  die 'Production checkout has tracked modifications'
fi

git -C "${STACK_DIR}" fetch --no-tags origin "refs/tags/${tag}:refs/tags/${tag}" \
  >/dev/null 2>&1 || die 'release tag fetch failed'
resolved_commit="$(git -C "${STACK_DIR}" rev-parse --verify "refs/tags/${tag}^{commit}" 2>/dev/null)" || \
  die 'release tag could not be resolved'
[[ "${resolved_commit}" == "${commit}" ]] || die 'manifest tag and commit do not match'

git -C "${STACK_DIR}" checkout --detach "${commit}" >/dev/null 2>&1 || \
  die 'release checkout failed'
[[ "$(git -C "${STACK_DIR}" rev-parse HEAD 2>/dev/null)" == "${commit}" ]] || \
  die 'detached checkout does not match manifest commit'

deploy_command="${STACK_DIR}/scripts/deploy-stack.sh"
[[ -x "${deploy_command}" ]] || die 'release deployment command is unavailable'

ENTRYPOINT_STAGE="deployment"
DEPLOY_RESULT_FILE="$(mktemp "${TMP_DIR%/}/ideaboard-production-deploy-result.XXXXXX")"
chmod 600 "${DEPLOY_RESULT_FILE}"

deploy_status=0
IDEABOARD_STACK_DIR="${STACK_DIR}" \
  ENV_FILE="${STACK_DIR}/docker/.env" \
  COMPOSE_PROJECT_NAME=ideaboard \
  IDEABOARD_DEPLOY_RESULT_FILE="${DEPLOY_RESULT_FILE}" \
  "${deploy_command}" update --manifest "${MANIFEST_FILE}" \
  > /dev/null 2>&1 || deploy_status=$?

if jq -e -s --argjson deploy_status "${deploy_status}" '
  length == 1
  and (.[0] | type == "object")
  and (.[0].result == "success" or .[0].result == "failure")
  and (.[0].stage | IN(
    "init", "resolve-images", "preflight", "pull", "start-postgres", "backup",
    "api-migration", "auth-migration", "up", "smoke", "complete", "unknown"
  ))
  and (.[0].recovery_hint | IN(
    "none",
    "inspect_release_state_and_service_logs_on_vps",
    "inspect_release_state_and_backups_before_manual_recovery"
  ))
  and (
    ($deploy_status == 0 and .[0].result == "success" and .[0].stage == "complete" and .[0].recovery_hint == "none")
    or ($deploy_status != 0 and .[0].result == "failure" and .[0].recovery_hint != "none")
  )
' "${DEPLOY_RESULT_FILE}" >/dev/null 2>&1; then
  jq -c '{result, stage, recovery_hint}' "${DEPLOY_RESULT_FILE}"
else
  printf '%s\n' \
    '{"result":"failure","stage":"unknown","recovery_hint":"inspect_release_state_and_service_logs_on_vps"}'
  (( deploy_status != 0 )) || deploy_status=1
fi

exit "${deploy_status}"
