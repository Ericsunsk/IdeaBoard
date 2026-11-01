#!/usr/bin/env bash

set +x
set -euo pipefail
export LC_ALL=C
umask 077

STACK_DIR="${IDEABOARD_PRODUCTION_STACK_DIR:-/opt/stacks/ideaboard}"
LOCK_FILE="${IDEABOARD_PRODUCTION_LOCK_FILE:-/run/lock/ideaboard-production-deploy.lock}"
TMP_DIR="${IDEABOARD_PRODUCTION_TMP_DIR:-/var/tmp}"
EXPECTED_REPOSITORY="${IDEABOARD_PRODUCTION_EXPECTED_REPOSITORY:-Ericsunsk/IdeaBoard}"
OWNER_ENV_FILE="${IDEABOARD_PRODUCTION_OWNER_ENV_FILE:-/var/lib/ideaboard/owner.env}"
CONFIG_FILE="${IDEABOARD_PRODUCTION_CONFIG_FILE:-/etc/ideaboard/deployment.json}"
BUNDLE_COMMAND="${IDEABOARD_PRODUCTION_BUNDLE_COMMAND:-/usr/local/libexec/ideaboard-deployment-bundle}"
DEPLOY_ENV=(env -u INFISICAL_TOKEN -u IDEABOARD_COMPOSE_OVERRIDE_FILE -u NPM_NETWORK_NAME IDEABOARD_SECRETS_SOURCE=infisical)
RECEIVE_DIR=""
MANIFEST_FILE=""
DEPLOY_RESULT_FILE=""
ENTRYPOINT_STAGE="bundle-validation"

die() {
  printf '{"result":"failure","stage":"%s","recovery_hint":"inspect_restricted_entrypoint_on_vps"}\n' \
    "${ENTRYPOINT_STAGE}"
  printf 'Production deployment rejected: %s\n' "$*" >&2
  exit 1
}

# shellcheck disable=SC2317,SC2329 # Invoked indirectly by the EXIT trap below.
cleanup() {
  if [[ -n "${RECEIVE_DIR}" ]]; then
    rm -rf -- "${RECEIVE_DIR}"
  fi
  if [[ -n "${DEPLOY_RESULT_FILE}" ]]; then
    rm -f -- "${DEPLOY_RESULT_FILE}"
  fi
}
trap cleanup EXIT

(( $# == 0 )) || die 'command arguments are not accepted'
[[ -z "${SSH_ORIGINAL_COMMAND:-}" ]] || die 'SSH commands are not accepted'
for required_command in jq flock mktemp docker infisical python3 cosign; do
  command -v "${required_command}" >/dev/null 2>&1 || die "missing command: ${required_command}"
done
[[ -d "${TMP_DIR}" ]] || die 'temporary directory is unavailable'
python3 - "${BUNDLE_COMMAND}" <<'PYTHON' || die 'installed deployment bundle verifier is unavailable'
import os, pathlib, stat, sys
try:
    info = pathlib.Path(sys.argv[1]).lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o022:
        raise ValueError()
except (OSError, ValueError):
    sys.exit(1)
PYTHON

# Host topology belongs to this root-owned entrypoint, never to Cloud secrets.
if [[ -e "${CONFIG_FILE}" || -L "${CONFIG_FILE}" ]]; then
  npm_network="$(python3 - "${CONFIG_FILE}" <<'PYTHON'
import json, os, pathlib, re, stat, sys
try:
    path = pathlib.Path(sys.argv[1])
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o022 or info.st_size > 4096:
        raise ValueError()
    def unique(pairs):
        result = dict(pairs)
        if len(result) != len(pairs):
            raise ValueError()
        return result
    config = json.loads(path.read_text(), object_pairs_hook=unique)
    if not isinstance(config, dict) or set(config) != {"npm_network"}:
        raise ValueError()
    network = config["npm_network"]
    if not isinstance(network, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}", network):
        raise ValueError()
    print(network)
except (OSError, ValueError, TypeError):
    sys.exit(1)
PYTHON
  )" || die 'invalid local deployment configuration'
  DEPLOY_ENV+=(IDEABOARD_COMPOSE_OVERRIDE_FILE=docker/compose.npm.yml "NPM_NETWORK_NAME=${npm_network}")
fi

# The only untrusted inputs are bounded headers and bytes, never SSH commands.
IFS= read -r -n 65 protocol || die 'deployment protocol header is missing'
[[ "${protocol}" == IDEABOARD_DEPLOY_V2 ]] || die 'unsupported deployment protocol'
IFS= read -r -n 8193 INFISICAL_TOKEN || die 'Production Infisical token is missing'
[[ -n "${INFISICAL_TOKEN}" && ${#INFISICAL_TOKEN} -le 8192 && "${INFISICAL_TOKEN}" != *[![:graph:]]* ]] || \
  die 'Production Infisical token is invalid'
IFS= read -r -n 4097 header || die 'deployment bundle header is missing'
(( ${#header} <= 4096 )) || die 'deployment bundle header is too large'

ENTRYPOINT_STAGE="lock"
mkdir -p "$(dirname "${LOCK_FILE}")"
exec 9>"${LOCK_FILE}"
flock -n 9 || die 'another Production deployment is running'
ENTRYPOINT_STAGE="bundle-validation"
RECEIVE_DIR="$(mktemp -d "${TMP_DIR%/}/ideaboard-production-bundle.XXXXXX")"
chmod 700 "${RECEIVE_DIR}"
# Python owns the binary framing so NUL bytes never enter shell variables.
python3 -c '
import json, pathlib, re, sys
try:
    def unique(pairs):
        value = dict(pairs)
        if len(value) != len(pairs): raise ValueError()
        return value
    header = json.loads(sys.argv[1], object_pairs_hook=unique)
    if not isinstance(header, dict) or set(header) != {"tag", "commit", "archive_bytes", "signature_bytes"}: raise ValueError()
    if not isinstance(header["tag"], str) or not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", header["tag"]): raise ValueError()
    if not isinstance(header["commit"], str) or not re.fullmatch(r"[0-9a-f]{40}", header["commit"]): raise ValueError()
    root = pathlib.Path(sys.argv[2])
    for field, name, maximum in [("archive_bytes", "deployment-bundle.tar", 67108864), ("signature_bytes", "deployment-bundle.sigstore.json", 1048576)]:
        size = header[field]
        if type(size) is not int or not 0 < size <= maximum: raise ValueError()
        data = sys.stdin.buffer.read(size)
        if len(data) != size: raise ValueError()
        (root / name).write_bytes(data)
    if sys.stdin.buffer.read(1): raise ValueError()
    (root / "header.json").write_text(json.dumps(header))
except (ValueError, TypeError, OSError):
    sys.exit(1)
' "${header}" "${RECEIVE_DIR}" || die 'invalid or truncated deployment bundle transport'
tag="$(jq -er .tag "${RECEIVE_DIR}/header.json")"
commit="$(jq -er .commit "${RECEIVE_DIR}/header.json")"
python3 "${BUNDLE_COMMAND}" verify \
  --archive "${RECEIVE_DIR}/deployment-bundle.tar" \
  --signature "${RECEIVE_DIR}/deployment-bundle.sigstore.json" \
  --repository "${EXPECTED_REPOSITORY}" --tag "${tag}" --commit "${commit}" \
  --output-dir "${RECEIVE_DIR}/content" >/dev/null 2>&1 || die 'deployment bundle signature or contents are invalid'

# Preserve the exact signed artifact and verified files for compatible rollback.
bundle_digest="$(python3 -c 'import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "${RECEIVE_DIR}/deployment-bundle.tar")"
install -d -m 700 "${STACK_DIR}/bundles/${tag}" "${STACK_DIR}/docker/releases"
bundle_dir="$(mktemp -d "${STACK_DIR}/bundles/${tag}/${bundle_digest}.XXXXXX")"
# Each deployment owns a private destination; previously verified releases are never overwritten.
for bundle_file in deployment-bundle.tar deployment-bundle.sigstore.json header.json content; do
  mv "${RECEIVE_DIR}/${bundle_file}" "${bundle_dir}/${bundle_file}"
done
release_dir="${bundle_dir}/content"
MANIFEST_FILE="${release_dir}/release-manifest.json"
(cd "${release_dir}" && python3 scripts/release-artifacts.py validate \
  --manifest "${MANIFEST_FILE}" --repository "${EXPECTED_REPOSITORY}" \
  --commit "${commit}" --tag "${tag}") >/dev/null 2>&1 || die 'signed component provenance is invalid'
deploy_command="${release_dir}/scripts/deploy-stack.sh"
inject_command="${release_dir}/scripts/infisical-run.sh"
[[ -x "${deploy_command}" && -x "${inject_command}" ]] || die 'signed deployment commands are unavailable'

# Existing PostgreSQL must retain its owner credential; never silently rotate it.
if [[ ! -f "${OWNER_ENV_FILE}" ]]; then
  database_volume="$(docker volume ls --filter 'name=^ideaboard_postgres-data$' --format '{{.Name}}')" || \
    die 'could not inspect the existing PostgreSQL volume'
  [[ -z "${database_volume}" ]] || die 'restore the existing PostgreSQL owner credential before deployment'
fi
install -d -m 700 "$(dirname "${OWNER_ENV_FILE}")"
[[ ! -f "${OWNER_ENV_FILE}" ]] || chmod 600 "${OWNER_ENV_FILE}"

ENTRYPOINT_STAGE="deployment"
DEPLOY_RESULT_FILE="$(mktemp "${TMP_DIR%/}/ideaboard-production-deploy-result.XXXXXX")"
chmod 600 "${DEPLOY_RESULT_FILE}"

# Cloud secrets may not redirect trusted files, persistent state, or verification.
DEPLOY_ENV+=(
  "IDEABOARD_STACK_DIR=${release_dir}"
  "IMAGE_ENV_FILE=${STACK_DIR}/docker/images.env"
  "RELEASE_STATE_DIR=${STACK_DIR}/docker/releases"
  "BACKUP_DIR=$(dirname "${STACK_DIR}")/$(basename "${STACK_DIR}")-backups"
  "IDEABOARD_DEPLOYMENT_BUNDLE_DIR=${release_dir}"
  "IDEABOARD_DEPLOYMENT_BUNDLE_DIGEST=sha256:${bundle_digest}"
  "IDEABOARD_DEPLOYMENT_BUNDLE_ARCHIVE=${bundle_dir}/deployment-bundle.tar"
  "IDEABOARD_DEPLOYMENT_BUNDLE_SIGNATURE=${bundle_dir}/deployment-bundle.sigstore.json"
  "ENV_FILE=${OWNER_ENV_FILE}"
  COMPOSE_PROJECT_NAME=ideaboard
  "IDEABOARD_DEPLOY_RESULT_FILE=${DEPLOY_RESULT_FILE}"
)
deploy_status=0
(
  cd "${release_dir}"
  IDEABOARD_STACK_DIR="${release_dir}" \
    IMAGE_ENV_FILE="${STACK_DIR}/docker/images.env" \
    RELEASE_STATE_DIR="${STACK_DIR}/docker/releases" \
    BACKUP_DIR="$(dirname "${STACK_DIR}")/$(basename "${STACK_DIR}")-backups" \
    IDEABOARD_DEPLOYMENT_BUNDLE_DIR="${release_dir}" \
    IDEABOARD_DEPLOYMENT_BUNDLE_DIGEST="sha256:${bundle_digest}" \
    IDEABOARD_DEPLOYMENT_BUNDLE_ARCHIVE="${bundle_dir}/deployment-bundle.tar" \
    IDEABOARD_DEPLOYMENT_BUNDLE_SIGNATURE="${bundle_dir}/deployment-bundle.sigstore.json" \
    ENV_FILE="${OWNER_ENV_FILE}" \
    COMPOSE_PROJECT_NAME=ideaboard \
    IDEABOARD_DEPLOY_RESULT_FILE="${DEPLOY_RESULT_FILE}" \
    INFISICAL_TOKEN="${INFISICAL_TOKEN}" \
    "${inject_command}" --env prod --path / --path /ai-proxy -- \
    "${DEPLOY_ENV[@]}" "${deploy_command}" deploy --manifest "${MANIFEST_FILE}"
) > /dev/null 2>&1 || deploy_status=$?

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
