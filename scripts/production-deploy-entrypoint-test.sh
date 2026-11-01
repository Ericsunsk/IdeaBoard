#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
entrypoint="${script_dir}/production-deploy-entrypoint.sh"
test_root="$(mktemp -d)"
test_bin="${test_root}/bin"

mkdir -p "${test_bin}"
cat > "${test_bin}/flock" <<'EOF'
#!/usr/bin/env bash
if [[ "${FLOCK_SHOULD_FAIL:-0}" == "1" ]]; then
  exit 1
fi
exit 0
EOF
chmod +x "${test_bin}/flock"
cat > "${test_bin}/docker" <<'EOF'
#!/usr/bin/env bash
[[ "$*" == "volume ls --filter name=^ideaboard_postgres-data$ --format {{.Name}}" ]] || exit 97
[[ "${VOLUME_INSPECTION_FAILS:-0}" != 1 ]] || exit 1
[[ "${EXISTING_DATABASE_VOLUME:-0}" != 1 ]] || printf '%s\n' ideaboard_postgres-data
exit 0
EOF
cat > "${test_bin}/infisical" <<'EOF'
#!/usr/bin/env bash
exit 97
EOF
chmod +x "${test_bin}/docker" "${test_bin}/infisical"

cleanup() {
  rm -rf -- "${test_root}"
}
trap cleanup EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_file_contains() {
  local file="$1"
  local expected="$2"

  grep -Fq -- "${expected}" "${file}" || fail "expected ${file} to contain ${expected}"
}

assert_directory_has_no_files() {
  local directory="$1"

  [[ -z "$(find "${directory}" -type f -print -quit)" ]] || \
    fail "expected ${directory} to contain no files"
}

create_checkout() {
  local fixture_root="$1"
  local seed="${fixture_root}/seed"
  local remote="${fixture_root}/origin.git"
  local stack="${fixture_root}/stack"

  mkdir -p "${seed}/scripts"
  git init -q "${seed}"
  git -C "${seed}" config user.email 'deploy-test@example.com'
  git -C "${seed}" config user.name 'Deploy Test'
  cat > "${seed}/scripts/deploy-stack.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" > "${DEPLOY_CAPTURE_DIR}/args"
printf '%s\n' "$3" > "${DEPLOY_CAPTURE_DIR}/manifest-path"
cp "$3" "${DEPLOY_CAPTURE_DIR}/manifest-copy"
printf '%s\n' "${IDEABOARD_STACK_DIR:-}" > "${DEPLOY_CAPTURE_DIR}/stack-dir"
printf '%s\n' "${ENV_FILE:-}" > "${DEPLOY_CAPTURE_DIR}/env-file"
printf '%s\n' "${COMPOSE_PROJECT_NAME:-}" > "${DEPLOY_CAPTURE_DIR}/compose-project"
[[ -z "${INFISICAL_TOKEN:-}" ]] || exit 96
printf 'RAW-DEPLOY-SECRET-MARKER\n' >&2
if [[ "${DEPLOY_SHOULD_FAIL:-0}" == "1" ]]; then
  if [[ "${SKIP_DEPLOY_RESULT:-0}" != "1" ]]; then
    printf '%s\n' \
      '{"result":"failure","stage":"smoke","recovery_hint":"inspect_release_state_and_service_logs_on_vps","secret":"RAW-RESULT-SECRET-MARKER"}' \
      > "${IDEABOARD_DEPLOY_RESULT_FILE}"
  fi
  exit 23
fi
if [[ "${SKIP_DEPLOY_RESULT:-0}" != "1" ]]; then
  printf '%s\n' \
    '{"result":"success","stage":"complete","recovery_hint":"none","secret":"RAW-RESULT-SECRET-MARKER"}' \
    > "${IDEABOARD_DEPLOY_RESULT_FILE}"
fi
EOF
  cat > "${seed}/scripts/infisical-run.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${INFISICAL_TOKEN:-}" == st.SYNTHETIC-INFISICAL-TOKEN ]] || exit 95
[[ "$PWD" == "$IDEABOARD_STACK_DIR" ]] || exit 93
[[ "$1 $2 $3 $4 $5 $6 $7" == '--env prod --path / --path /ai-proxy --' ]] || exit 94
printf 'prod root+ai-proxy injected\n' > "${DEPLOY_CAPTURE_DIR}/injection"
shift 7
exec "$@"
EOF
  chmod +x "${seed}/scripts/deploy-stack.sh"
  chmod +x "${seed}/scripts/infisical-run.sh"
  git -C "${seed}" add scripts/deploy-stack.sh scripts/infisical-run.sh
  git -C "${seed}" commit -qm 'test release'
  git -C "${seed}" tag v1.2.3
  git -C "${seed}" tag release-1.2.3
  printf 'next release\n' > "${seed}/NEXT_RELEASE"
  git -C "${seed}" add NEXT_RELEASE
  git -C "${seed}" commit -qm 'next test release'
  git -C "${seed}" tag v1.2.4
  git clone -q --bare "${seed}" "${remote}"
  git clone -q "${remote}" "${stack}"
  printf '%s\n' "${stack}"
}

write_manifest() {
  local destination="$1"
  local commit="$2"
  local digest
  digest="$(printf 'a%.0s' {1..64})"

  cat > "${destination}" <<EOF
{
  "repository": "Ericsunsk/IdeaBoard",
  "tag": "v1.2.3",
  "commit": "${commit}",
  "images": {
    "api": "ghcr.io/ericsunsk/ideaboard/ideaboard-api@sha256:${digest}",
    "api-migrate": "ghcr.io/ericsunsk/ideaboard/ideaboard-api-migrate@sha256:${digest}",
    "auth": "ghcr.io/ericsunsk/ideaboard/ideaboard-auth@sha256:${digest}",
    "auth-migrate": "ghcr.io/ericsunsk/ideaboard/ideaboard-auth-migrate@sha256:${digest}",
    "realtime": "ghcr.io/ericsunsk/ideaboard/ideaboard-realtime@sha256:${digest}"
  }
}
EOF
}

setup_release_fixture() {
  local case_name="$1"

  fixture_root="${test_root}/${case_name}"
  capture="${fixture_root}/capture"
  manifest="${fixture_root}/release-manifest.json"
  mkdir -p "${fixture_root}" "${capture}" "${fixture_root}/tmp"
  stack="$(create_checkout "${fixture_root}")"
  commit="$(git -C "${stack}" rev-parse 'refs/tags/v1.2.3^{commit}')"
  write_manifest "${manifest}" "${commit}"
}

run_entrypoint_raw() {
  env \
    PATH="${test_bin}:${PATH}" \
    DEPLOY_CAPTURE_DIR="${capture}" \
    IDEABOARD_PRODUCTION_STACK_DIR="${stack}" \
    IDEABOARD_PRODUCTION_LOCK_FILE="${fixture_root}/deploy.lock" \
    IDEABOARD_PRODUCTION_TMP_DIR="${fixture_root}/tmp" \
    IDEABOARD_PRODUCTION_OWNER_ENV_FILE="${fixture_root}/state/owner.env" \
    bash "${entrypoint}" "$@"
}

run_entrypoint() {
  {
    printf '%s\n' IDEABOARD_DEPLOY_V1 st.SYNTHETIC-INFISICAL-TOKEN
    cat
  } | run_entrypoint_raw "$@"
}

test_valid_manifest_reaches_deploy_command() {
  local deployed_manifest_path output

  setup_release_fixture 'valid'
  output="$(run_entrypoint < "${manifest}" 2>&1)"

  assert_file_contains "${capture}/args" 'update --manifest '
  cmp -s "${manifest}" "${capture}/manifest-copy" || fail 'deploy command received the wrong manifest'
  deployed_manifest_path="$(< "${capture}/manifest-path")"
  [[ ! -e "${deployed_manifest_path}" ]] || fail 'temporary manifest was not removed after success'
  assert_file_contains "${capture}/stack-dir" "${stack}"
  assert_file_contains "${capture}/env-file" "${fixture_root}/state/owner.env"
  assert_file_contains "${capture}/injection" 'prod root+ai-proxy injected'
  assert_file_contains "${capture}/compose-project" 'ideaboard'
  [[ "${output}" == '{"result":"success","stage":"complete","recovery_hint":"none"}' ]] || \
    fail 'valid deployment did not return the allow-listed automation result'
  [[ "${output}" != *'RAW-DEPLOY-SECRET-MARKER'* ]] || fail 'raw deploy output reached the SSH caller'
  [[ "${output}" != *'RAW-RESULT-SECRET-MARKER'* ]] || fail 'non-allow-listed result data reached the SSH caller'
  [[ "${output}" != *'SYNTHETIC-INFISICAL-TOKEN'* ]] || fail 'Infisical token reached caller output'
  if grep -rFq 'SYNTHETIC-INFISICAL-TOKEN' "${capture}" "${fixture_root}/tmp"; then
    fail 'Infisical token reached a manifest, result or capture file'
  fi
}

test_missing_malformed_and_oversized_tokens_are_rejected() {
  local token output
  setup_release_fixture 'invalid-credential'
  for token in '' 'invalid token' $'token\r' "$(printf '%8193s' '' | tr ' ' x)"; do
    if output="$({ printf '%s\n' IDEABOARD_DEPLOY_V1 "$token"; cat "${manifest}"; } | run_entrypoint_raw 2>&1)"; then
      fail 'invalid token reached deployment'
    fi
    [[ ! -e "${capture}/args" ]] || fail 'invalid token invoked deployment'
    [[ "$output" != *"$token"* || -z "$token" ]] || fail 'invalid token was printed'
    assert_directory_has_no_files "${fixture_root}/tmp"
  done
  if run_entrypoint_raw < "${manifest}" >/dev/null 2>&1; then
    fail 'legacy manifest-only protocol bypassed required token'
  fi
}

test_existing_database_requires_persisted_owner() {
  setup_release_fixture 'existing-owner'
  if EXISTING_DATABASE_VOLUME=1 run_entrypoint < "${manifest}" >/dev/null 2>&1; then
    fail 'existing PostgreSQL accepted a newly generated owner password'
  fi
  [[ ! -e "${capture}/args" ]] || fail 'missing owner credential reached deploy command'
  mkdir -p "${fixture_root}/state"
  printf 'POSTGRES_PASSWORD=existing-owner-password\n' > "${fixture_root}/state/owner.env"
  EXISTING_DATABASE_VOLUME=1 run_entrypoint < "${manifest}" >/dev/null || fail 'persisted owner should allow existing DB deployment'
  assert_file_contains "${fixture_root}/state/owner.env" 'POSTGRES_PASSWORD=existing-owner-password'
}

test_volume_inspection_fails_closed() {
  setup_release_fixture 'volume-inspection-failure'
  if VOLUME_INSPECTION_FAILS=1 run_entrypoint < "${manifest}" >/dev/null 2>&1; then
    fail 'volume inspection failure was treated as fresh database'
  fi
  [[ ! -e "${capture}/args" ]] || fail 'volume inspection failure reached deployment'
}

test_invalid_tag_is_rejected_before_deploy() {
  local output

  setup_release_fixture 'invalid-tag'
  jq '.tag = "release-1.2.3"' "${manifest}" > "${manifest}.tmp"
  mv "${manifest}.tmp" "${manifest}"

  if output="$(run_entrypoint < "${manifest}" 2>/dev/null)"; then
    fail 'invalid tag reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'invalid tag invoked deploy command'
  [[ "${output}" == '{"result":"failure","stage":"manifest-validation","recovery_hint":"inspect_restricted_entrypoint_on_vps"}' ]] || \
    fail 'manifest validation failure did not return the allow-listed automation result'
}

test_wrong_repository_is_rejected_before_deploy() {
  setup_release_fixture 'wrong-repository'
  jq '.repository = "attacker/IdeaBoard"' "${manifest}" > "${manifest}.tmp"
  mv "${manifest}.tmp" "${manifest}"

  if run_entrypoint < "${manifest}"; then
    fail 'wrong repository reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'wrong repository invoked deploy command'
}

test_invalid_image_digest_is_rejected_before_deploy() {
  setup_release_fixture 'invalid-digest'
  jq '.images.api = "ghcr.io/ericsunsk/ideaboard/ideaboard-api:latest"' \
    "${manifest}" > "${manifest}.tmp"
  mv "${manifest}.tmp" "${manifest}"

  if run_entrypoint < "${manifest}"; then
    fail 'invalid image digest reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'invalid image digest invoked deploy command'
}

test_oversized_manifest_is_rejected_and_removed() {
  setup_release_fixture 'oversized'
  printf '%65537s' '' >> "${manifest}"

  if run_entrypoint < "${manifest}"; then
    fail 'oversized manifest reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'oversized manifest invoked deploy command'
  assert_directory_has_no_files "${fixture_root}/tmp"
}

test_invalid_commit_is_rejected_before_deploy() {
  setup_release_fixture 'invalid-commit'
  write_manifest "${manifest}" "${commit:0:12}"

  if run_entrypoint < "${manifest}"; then
    fail 'abbreviated commit reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'abbreviated commit invoked deploy command'
  assert_directory_has_no_files "${fixture_root}/tmp"
}

test_malformed_empty_and_trailing_input_are_rejected() {
  local output

  setup_release_fixture 'invalid-json'
  output="${fixture_root}/output"

  for invalid_input in '' '{"tag":' '{"tag":"v1.2.3"}TRAILING-SECRET-MARKER'; do
    printf '%s' "${invalid_input}" > "${manifest}"
    if run_entrypoint < "${manifest}" > "${output}" 2>&1; then
      fail 'invalid JSON input reached deployment'
    fi
    [[ ! -e "${capture}/args" ]] || fail 'invalid JSON input invoked deploy command'
    assert_directory_has_no_files "${fixture_root}/tmp"
    if grep -Fq 'TRAILING-SECRET-MARKER' "${output}"; then
      fail 'deployment entrypoint printed manifest contents'
    fi
  done
}

test_tag_commit_mismatch_is_rejected_before_deploy() {
  local wrong_commit

  setup_release_fixture 'tag-mismatch'
  wrong_commit="$(git -C "${stack}" rev-parse 'refs/tags/v1.2.4^{commit}')"
  write_manifest "${manifest}" "${wrong_commit}"

  if run_entrypoint < "${manifest}"; then
    fail 'tag and commit mismatch reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'tag and commit mismatch invoked deploy command'
  assert_directory_has_no_files "${fixture_root}/tmp"
}

test_dirty_checkout_is_rejected_before_deploy() {
  setup_release_fixture 'dirty-checkout'
  printf '# tracked modification\n' >> "${stack}/scripts/deploy-stack.sh"

  if run_entrypoint < "${manifest}"; then
    fail 'dirty checkout reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'dirty checkout invoked deploy command'
  assert_directory_has_no_files "${fixture_root}/tmp"
}

test_lock_contention_is_rejected_before_deploy() {
  setup_release_fixture 'lock-contention'

  if FLOCK_SHOULD_FAIL=1 run_entrypoint < "${manifest}"; then
    fail 'lock contention reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'lock contention invoked deploy command'
  assert_directory_has_no_files "${fixture_root}/tmp"
}

test_caller_commands_are_rejected() {
  setup_release_fixture 'caller-command'

  if run_entrypoint unexpected < "${manifest}"; then
    fail 'caller argument reached deployment'
  fi

  if SSH_ORIGINAL_COMMAND='cat /etc/passwd' run_entrypoint < "${manifest}"; then
    fail 'SSH original command reached deployment'
  fi

  [[ ! -e "${capture}/args" ]] || fail 'caller command invoked deploy command'
  assert_directory_has_no_files "${fixture_root}/tmp"
}

test_temporary_manifest_is_removed_when_deploy_fails() {
  local deployed_manifest_path output

  setup_release_fixture 'deploy-failure'

  if output="$(DEPLOY_SHOULD_FAIL=1 run_entrypoint < "${manifest}" 2>&1)"; then
    fail 'deploy command failure returned success'
  fi

  deployed_manifest_path="$(< "${capture}/manifest-path")"
  [[ ! -e "${deployed_manifest_path}" ]] || fail 'temporary manifest was not removed after deploy failure'
  assert_directory_has_no_files "${fixture_root}/tmp"
  [[ "${output}" == '{"result":"failure","stage":"smoke","recovery_hint":"inspect_release_state_and_service_logs_on_vps"}' ]] || \
    fail 'failed deployment did not return the allow-listed automation result'
  [[ "${output}" != *'RAW-DEPLOY-SECRET-MARKER'* ]] || fail 'raw deploy failure output reached the SSH caller'
  [[ "${output}" != *'RAW-RESULT-SECRET-MARKER'* ]] || fail 'non-allow-listed failure result data reached the SSH caller'
}

test_missing_automation_result_fails_closed() {
  local output

  setup_release_fixture 'missing-result'

  if output="$(SKIP_DEPLOY_RESULT=1 run_entrypoint < "${manifest}" 2>&1)"; then
    fail 'deployment without an automation result returned success'
  fi
  [[ "${output}" == '{"result":"failure","stage":"unknown","recovery_hint":"inspect_release_state_and_service_logs_on_vps"}' ]] || \
    fail 'missing automation result did not return the fail-closed result'
  [[ "${output}" != *'RAW-DEPLOY-SECRET-MARKER'* ]] || fail 'raw output reached the SSH caller'
}

test_valid_manifest_reaches_deploy_command
test_missing_malformed_and_oversized_tokens_are_rejected
test_existing_database_requires_persisted_owner
test_volume_inspection_fails_closed
test_invalid_tag_is_rejected_before_deploy
test_wrong_repository_is_rejected_before_deploy
test_invalid_image_digest_is_rejected_before_deploy
test_oversized_manifest_is_rejected_and_removed
test_invalid_commit_is_rejected_before_deploy
test_malformed_empty_and_trailing_input_are_rejected
test_tag_commit_mismatch_is_rejected_before_deploy
test_dirty_checkout_is_rejected_before_deploy
test_lock_contention_is_rejected_before_deploy
test_caller_commands_are_rejected
test_temporary_manifest_is_removed_when_deploy_fails
test_missing_automation_result_fails_closed
printf 'Production deploy entrypoint contract tests passed\n'
