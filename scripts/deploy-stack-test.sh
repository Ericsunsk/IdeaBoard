#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
deploy_script="${script_dir}/deploy-stack.sh"
bootstrap_script="${script_dir}/bootstrap-vps.sh"
fixture_dir="$(mktemp -d)"
trap 'rm -rf -- "${fixture_dir}"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_fails_with() {
  local expected="$1"
  shift
  local output

  if output="$("$@" 2>&1)"; then
    fail "command unexpectedly succeeded: $*"
  fi
  [[ "$output" == *"$expected"* ]] || fail "expected failure to contain: $expected"
}

assert_fails_with \
  'update requires --manifest <path-or-url>' \
  env IDEABOARD_STACK_DIR="${fixture_dir}" "${deploy_script}" update
[[ ! -e "${fixture_dir}/docker/.env" ]] || fail 'missing manifest must fail before creating an environment file'

assert_fails_with \
  'update requires --manifest <path-or-url>' \
  env IDEABOARD_STACK_DIR="${fixture_dir}" "${deploy_script}"

assert_fails_with \
  'deploy requires --manifest <path-or-url>' \
  env IDEABOARD_STACK_DIR="${fixture_dir}" "${deploy_script}" deploy

assert_fails_with \
  '--manifest requires a path or URL' \
  env IDEABOARD_STACK_DIR="${fixture_dir}" "${deploy_script}" update --manifest

assert_fails_with \
  '--ref <tag-or-commit> is required' \
  "${bootstrap_script}"
assert_fails_with \
  '--manifest <path-or-url> is required' \
  "${bootstrap_script}" --ref v1.2.3
assert_fails_with \
  '--ref must be a strict SemVer tag or full Git commit SHA' \
  "${bootstrap_script}" --ref main --manifest "${fixture_dir}/release-manifest.json"

checkout_fixture="${fixture_dir}/checkout"
mkdir -p "${checkout_fixture}/docker"
cp "${repo_root}/docker/.env.example" "${checkout_fixture}/docker/.env.example"
printf 'services: {}\n' > "${checkout_fixture}/docker/compose.yml"
git init -q "${checkout_fixture}"
git -C "${checkout_fixture}" config user.email 'deploy-test@example.com'
git -C "${checkout_fixture}" config user.name 'Deploy Test'
git -C "${checkout_fixture}" add docker
git -C "${checkout_fixture}" commit -qm 'test checkout'
digest="$(printf 'a%.0s' {1..64})"
cat > "${fixture_dir}/release-manifest.json" <<EOF
{
  "tag": "v1.2.3",
  "commit": "$(printf 'b%.0s' {1..40})",
  "images": {
    "api": "ghcr.io/ericsunsk/ideaboard/ideaboard-api@sha256:${digest}",
    "api-migrate": "ghcr.io/ericsunsk/ideaboard/ideaboard-api-migrate@sha256:${digest}",
    "auth": "ghcr.io/ericsunsk/ideaboard/ideaboard-auth@sha256:${digest}",
    "auth-migrate": "ghcr.io/ericsunsk/ideaboard/ideaboard-auth-migrate@sha256:${digest}"
  }
}
EOF
assert_fails_with \
  'does not match release manifest commit' \
  env IDEABOARD_STACK_DIR="${checkout_fixture}" \
  "${deploy_script}" update --manifest "${fixture_dir}/release-manifest.json"

for removed_contract in \
  IDEABOARD_BRANCH \
  IDEABOARD_UPDATE_TAG \
  perform_auto_update \
  detect_update_tag \
  resolve_digest_for_image \
  needs_auto_update; do
  if grep -R -Fq -- "$removed_contract" \
    "${deploy_script}" "${bootstrap_script}" "${repo_root}/docker/.env.example"; then
    fail "mutable image fallback remains: ${removed_contract}"
  fi
done

(
  functions_file="$fixture_dir/function.sh"
  sed -n '/^stage_workspace_uploads() (/,/^)/p' "${deploy_script}" > "$functions_file"
  source "$functions_file"
  log_file="$fixture_dir/log"

  env_value() { printf /app; }
  docker_compose() {
    if [[ "$1" == ps ]]; then
      [[ "$case_name" != fresh ]] || return 0
      printf test-container
    else
      printf 'compose %s\n' "$*" >> "$log_file"
      [[ "$case_name" != stop-failed ]]
    fi
  }
  docker() {
    printf 'docker %s\n' "$*" >> "$log_file"
    if [[ "$2" == -a ]]; then
      [[ "$case_name" != import-failed ]] || return 1
      [[ "$(cat)" == archive ]] || return 1
    elif [[ "$case_name" == missing ]]; then
      echo 'Could not find the file /app/ws-uploads in container test-container' >&2
      return 1
    elif [[ "$case_name" == failed ]]; then
      echo 'permission denied' >&2
      return 1
    elif [[ "$case_name" == snapshot-failed && -s "$log_file" ]] &&
         grep -Fq 'compose stop api' "$log_file"; then
      return 1
    else
      printf archive
    fi
  }
  case_name=success
  : > "$log_file"
  stage_workspace_uploads
  [[ "$(cat "$log_file")" == $'docker cp test-container:/app/ws-uploads -\ncompose stop api\ndocker cp test-container:/app/ws-uploads -\ndocker cp -a - test-container:/app/uploads' ]]
  : > "$log_file"
  case_name=fresh
  stage_workspace_uploads
  [[ ! -s "$log_file" ]]
  case_name=missing
  stage_workspace_uploads
  [[ "$(cat "$log_file")" == 'docker cp test-container:/app/ws-uploads -' ]]
  for case_name in failed stop-failed snapshot-failed import-failed; do
    : > "$log_file"
    if stage_workspace_uploads 2>/dev/null; then
      fail "workspace object staging must reject $case_name"
    fi
  done
  for entrypoint in run_deploy_transaction '  migrate)'; do
    pipeline="$(sed -n "/^${entrypoint}/,/^}/p" "$deploy_script")"
    [[ "$pipeline" == *run_stack_migrations* ]] ||
      fail "$entrypoint must use the shared migration pipeline"
  done
)

(
  functions_file="$fixture_dir/migrations.sh"
  sed -n '/^run_stack_migrations() {/,/^}/p' "$deploy_script" > "$functions_file"
  cat >> "$functions_file" <<'BASH'
record_step() {
  printf '%s:%s\n' "$DEPLOY_STAGE" "$1" >> "$MIGRATION_TEST_LOG"
  [[ "$1" != "$MIGRATION_TEST_FAILURE" ]]
}
docker_compose() { record_step postgres; }
wait_for_postgres() { record_step ready; }
stage_workspace_uploads() { record_step uploads; }
backup_postgres() { record_step database-backup; }
backup_api_uploads() { record_step uploads-backup; }
run_api_migration() { record_step api; }
run_auth_migration() { record_step auth; }
DEPLOY_BACKUP_GATE=pending
DEPLOY_MIGRATION_GATE=pending
DEPLOY_MIGRATION_STARTED=0
trap 'printf "gates:%s:%s:%s\n" "$DEPLOY_BACKUP_GATE" "$DEPLOY_MIGRATION_GATE" "$DEPLOY_MIGRATION_STARTED" >> "$MIGRATION_TEST_LOG"' EXIT
run_stack_migrations
BASH
  expected_steps=$'start-postgres:postgres\nstart-postgres:ready\nstage-uploads:uploads\nbackup:database-backup\nbackup:uploads-backup\napi-migration:api\nauth-migration:auth'
  for failed_step in none postgres ready uploads database-backup uploads-backup api auth; do
    log_file="$fixture_dir/migration-$failed_step.log"
    if env MIGRATION_TEST_LOG="$log_file" MIGRATION_TEST_FAILURE="$failed_step" bash -e "$functions_file"; then
      [[ "$failed_step" == none ]] || fail "migration must stop at $failed_step"
    else
      [[ "$failed_step" != none ]] || fail 'shared migration pipeline failed'
    fi
    expected_prefix=''
    while IFS= read -r step; do
      expected_prefix+="${expected_prefix:+$'\n'}$step"
      [[ "${step#*:}" != "$failed_step" ]] || break
    done <<< "$expected_steps"
    case "$failed_step" in
      none) gates='gates:passed:passed:1' ;;
      api|auth) gates='gates:passed:pending:1' ;;
      *) gates='gates:pending:pending:0' ;;
    esac
    [[ "$(cat "$log_file")" == "$expected_prefix"$'\n'"$gates" ]] ||
      fail "wrong migration order or gate state at $failed_step"
  done
)

printf 'Deploy stack contract tests passed\n'
