#!/usr/bin/env bash

# shellcheck disable=SC2329 # Test doubles are called by dynamically extracted functions.
# shellcheck disable=SC2030,SC2031 # Each subshell independently initializes its fixture environment.
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
cp "${repo_root}/docker/images.env.example" "${checkout_fixture}/docker/images.env.example"
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
    "auth-migrate": "ghcr.io/ericsunsk/ideaboard/ideaboard-auth-migrate@sha256:${digest}",
    "realtime": "ghcr.io/ericsunsk/ideaboard/ideaboard-realtime@sha256:${digest}"
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
  # shellcheck source=/dev/null
  source "$functions_file"
  log_file="$fixture_dir/log"

  # shellcheck disable=SC2317 # Called by the sourced production functions.
  env_value() { printf /app; }
  # shellcheck disable=SC2317 # Called by the sourced production functions.
  docker_compose() {
    if [[ "$1" == ps ]]; then
      [[ "$case_name" != fresh ]] || return 0
      printf test-container
    else
      printf 'compose %s\n' "$*" >> "$log_file"
      [[ "$case_name" != stop-failed ]]
    fi
  }
  # shellcheck disable=SC2317 # Called by the sourced production functions.
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

(
  functions_file="$fixture_dir/dotenvx.sh"
  {
    sed -n '/^dotenvx_encrypted_file() {/,/^}/p' "$deploy_script"
    sed -n '/^prepare_dotenvx_env() {/,/^}/p' "$deploy_script"
  } > "$functions_file"
  # shellcheck disable=SC2317 # Called by the sourced production functions.
  die() { echo "Error: $*" >&2; exit 1; }
  # shellcheck source=/dev/null
  source "$functions_file"
  dx_case="$fixture_dir/dotenvx"
  mkdir -p "$dx_case"
  printf 'PUBLIC_ORIGIN=https://example.com\n' >"$dx_case/plain.env"
  printf 'DOTENV_PUBLIC_KEY="abc"\nPUBLIC_ORIGIN=encrypted:xyz\n' >"$dx_case/enc.env"
  printf 'key-material\n' >"$dx_case/.env.keys"

  dotenvx_encrypted_file "$dx_case/plain.env" && fail 'plaintext env must not detect as dotenvx-encrypted'
  dotenvx_encrypted_file "$dx_case/missing.env" && fail 'missing env must not detect as dotenvx-encrypted'
  dotenvx_encrypted_file "$dx_case/enc.env" || fail 'encrypted env must detect as dotenvx-encrypted'

  # shellcheck disable=SC2317 # Called by the sourced production functions.
  dotenvx() {
    printf 'dotenvx %s\n' "$*" >>"$dx_case/log"
    [[ "$1" == decrypt ]] || return 1
    printf 'PUBLIC_ORIGIN=https://example.com\n'
  }

  ENV_FILE="$dx_case/plain.env"
  RENDERED_SECRETS_ENV=""
  unset DOTENV_PRIVATE_KEY DOTENV_KEYS_FILE || true
  prepare_dotenvx_env
  [[ "$ENV_FILE" == "$dx_case/plain.env" ]] || fail 'plaintext env must pass through untouched'
  [[ -z "$RENDERED_SECRETS_ENV" ]] || fail 'plaintext env must not render a temp file'

  ENV_FILE="$dx_case/enc.env"
  RENDERED_SECRETS_ENV=""
  export DOTENV_PRIVATE_KEY='test-private-key'
  : >"$dx_case/log"
  prepare_dotenvx_env
  grep -Fq 'decrypt -f' "$dx_case/log" || fail 'encrypted env must decrypt via dotenvx'
  grep -Fq -- '-fk' "$dx_case/log" && fail 'DOTENV_PRIVATE_KEY must win over key files without -fk'
  [[ "$ENV_FILE" != "$dx_case/enc.env" ]] || fail 'encrypted env must switch to the decrypted temp file'
  [[ "$(cat "$ENV_FILE")" == 'PUBLIC_ORIGIN=https://example.com' ]] || fail 'decrypted temp file has wrong content'
  # Select stat syntax explicitly: a failed GNU -f can still emit output.
  if [[ "$(uname)" == Darwin ]]; then
    decrypted_mode="$(stat -f %Lp "$ENV_FILE")"
  else
    decrypted_mode="$(stat -c %a "$ENV_FILE")"
  fi
  [[ "$decrypted_mode" == 600 ]] || fail 'decrypted temp file must be mode 600'
  rm -f -- "$ENV_FILE"
  RENDERED_SECRETS_ENV=""

  unset DOTENV_PRIVATE_KEY
  export DOTENV_KEYS_FILE="$dx_case/.env.keys"
  ENV_FILE="$dx_case/enc.env"
  : >"$dx_case/log"
  prepare_dotenvx_env
  grep -Fq -- "-fk $dx_case/.env.keys" "$dx_case/log" || fail 'key file must be passed via -fk'
  rm -f -- "$ENV_FILE"
  RENDERED_SECRETS_ENV=""
  unset DOTENV_KEYS_FILE

  # Decrypt can emit partial plaintext before failing. No file may survive
  # either key-selection path, including before the caller installs its trap.
  mkdir -p "$dx_case/failure-tmp"
  # shellcheck disable=SC2317 # Called by the sourced production functions.
  dotenvx() { printf 'POSTGRES_PASSWORD=synthetic-partial-output\n'; return 1; }
  for key_source in environment file; do
    export DOTENV_KEYS_FILE="$dx_case/.env.keys"
    if [[ "$key_source" == environment ]]; then
      export DOTENV_PRIVATE_KEY=test-private-key
    else
      unset DOTENV_PRIVATE_KEY
    fi
    ENV_FILE="$dx_case/enc.env"
    if ( TMPDIR="$dx_case/failure-tmp" prepare_dotenvx_env ) 2>/dev/null; then
      fail "partial decrypt failure with $key_source key must fail closed"
    fi
    [[ -z "$(ls -A "$dx_case/failure-tmp")" ]] || fail 'failed decrypt must remove partial plaintext'
    [[ "$ENV_FILE" == "$dx_case/enc.env" ]] || fail 'failed decrypt must preserve the selected encrypted source'
  done
  unset DOTENV_PRIVATE_KEY DOTENV_KEYS_FILE

  rm -f "$dx_case/.env.keys"
  ENV_FILE="$dx_case/enc.env"
  if ( prepare_dotenvx_env ) 2>/dev/null; then
    fail 'encrypted env without any private key must fail'
  fi
  [[ "$ENV_FILE" == "$dx_case/enc.env" ]] || fail 'failed decrypt must keep the original env file'

  hide_bin="$dx_case/hide-bin"
  mkdir -p "$hide_bin"
  for tool in mktemp dirname chmod rm grep; do
    ln -s "$(command -v "$tool")" "$hide_bin/$tool"
  done
  if output="$(env -i PATH="$hide_bin" DOTENV_PRIVATE_KEY='test-private-key' ENV_FILE="$dx_case/enc.env" "$(command -v bash)" -c "die() { echo \"Error: \$*\" >&2; exit 1; }; source '$functions_file'; prepare_dotenvx_env" 2>&1)"; then
    fail 'missing dotenvx must fail closed'
  fi
  [[ "$output" == *'dotenvx is not installed'* ]] || fail 'missing dotenvx must point at onboarding docs'
)

printf 'Deploy stack dotenvx contract tests passed\n'

(
  for name in env_value env_or_file_value set_env_value restore_env_snapshot cleanup_rendered_secrets docker_compose; do
    sed -n "/^${name}() {/,/^}/p" "$deploy_script" >> "$fixture_dir/image-functions.sh"
  done
  # shellcheck source=/dev/null
  source "$fixture_dir/image-functions.sh"
  ENV_FILE="$fixture_dir/secret-plain.env"
  BASE_ENV_FILE="$fixture_dir/secret-encrypted.env"
  IMAGE_ENV_FILE="$fixture_dir/images.env"
  printf 'POSTGRES_PASSWORD=encrypted:unchanged\n' > "$BASE_ENV_FILE"
  printf 'POSTGRES_PASSWORD=test-db-password\n' > "$ENV_FILE"
  printf 'IDEABOARD_API_IMAGE=old-ref\n' > "$IMAGE_ENV_FILE"
  DEPLOY_ENV_SNAPSHOT="$fixture_dir/image-snapshot"
  cp "$IMAGE_ENV_FILE" "$DEPLOY_ENV_SNAPSHOT"
  set_env_value IDEABOARD_API_IMAGE new-ref
  RENDERED_SECRETS_ENV="$ENV_FILE"
  cleanup_rendered_secrets
  [[ "$(env_value IDEABOARD_API_IMAGE)" == new-ref ]] || fail 'successful release refs must survive decrypted env cleanup'
  [[ "$(cat "$BASE_ENV_FILE")" == POSTGRES_PASSWORD=encrypted:unchanged ]] || fail 'release must not rewrite frozen secrets'
  restore_env_snapshot
  [[ "$(env_value IDEABOARD_API_IMAGE)" == old-ref ]] || fail 'failed release must restore refs'
  [[ "$ENV_FILE" != "$BASE_ENV_FILE" ]] || fail 'rollback must not select encrypted secrets as runtime env'

  printf 'POSTGRES_PASSWORD=local-owner-password\nAPI_PG_PASSWORD=offline-runtime-password\n' > "$ENV_FILE"
  export IDEABOARD_SECRETS_SOURCE=infisical POSTGRES_PASSWORD=untrusted-cloud-owner API_PG_PASSWORD=cloud-runtime-password
  [[ "$(env_value POSTGRES_PASSWORD)" == local-owner-password ]] || fail 'Cloud deployment must read owner credentials only from its local source'
  [[ "$(env_value API_PG_PASSWORD)" == cloud-runtime-password ]] || fail 'Cloud deployment must read runtime secrets only from injected values'
  export COMPOSE_FILE=compose.yml ROOT_DIR="$fixture_dir"
  # shellcheck disable=SC2317 # Called by the sourced production functions.
  docker() {
    [[ "$POSTGRES_PASSWORD" == local-owner-password ]] || fail 'Compose must receive the local owner password'
    [[ "$API_PG_PASSWORD" == cloud-runtime-password ]] || fail 'Compose must receive Cloud runtime secrets'
    printf '%s\n' "$@" > "$fixture_dir/compose-args"
  }
  docker_compose config
  grep -Fxq "$IMAGE_ENV_FILE" "$fixture_dir/compose-args" || fail 'Compose must consume persistent image refs'
  if grep -Fxq "$ENV_FILE" "$fixture_dir/compose-args"; then
    fail 'Cloud Compose must never consume offline secrets'
  fi
)

printf 'Deploy stack image persistence and Cloud source tests passed\n'

(
  for name in ensure_env_file prepare_dotenvx_env dotenvx_encrypted_file cleanup_rendered_secrets env_value set_env_value is_placeholder generate_secret; do
    sed -n "/^${name}() {/,/^}/p" "$deploy_script" >> "$fixture_dir/owner-functions.sh"
  done
  # shellcheck source=/dev/null
  source "$fixture_dir/owner-functions.sh"
  # shellcheck disable=SC2317 # Called by the sourced production functions.
  die() { echo "Error: $*" >&2; exit 1; }
  ENV_FILE="$fixture_dir/local-owner.env"
  export ENV_EXAMPLE="$repo_root/docker/.env.example"
  RENDERED_SECRETS_ENV=""
  export IDEABOARD_SECRETS_SOURCE=infisical API_PG_PASSWORD=cloud-runtime-password
  unset POSTGRES_PASSWORD
  ensure_env_file >/dev/null
  owner_password="$(env_value POSTGRES_PASSWORD)"
  is_placeholder "$owner_password" && fail 'first Cloud deploy must generate a local owner password'
  ensure_env_file >/dev/null
  [[ "$(env_value POSTGRES_PASSWORD)" == "$owner_password" ]] || fail 'subsequent deploys must retain the owner password'
  grep -Fq 'API_PG_PASSWORD=replace-with-' "$ENV_FILE" || fail 'Cloud deployment must not generate or overwrite local business secrets'

  printf 'DOTENV_PUBLIC_KEY="abc"\nPOSTGRES_PASSWORD=encrypted:xyz\n' > "$ENV_FILE"
  frozen_hash="$(cksum < "$ENV_FILE")"
  export DOTENV_PRIVATE_KEY=synthetic-key
  # shellcheck disable=SC2317 # Called by the sourced production functions.
  dotenvx() { printf 'POSTGRES_PASSWORD=frozen-owner-password\nAPI_PG_PASSWORD=offline-runtime-password\n'; }
  ensure_env_file >/dev/null
  [[ "$(env_value POSTGRES_PASSWORD)" == frozen-owner-password ]] || fail 'Cloud deployment must decrypt the local owner password'
  [[ "$(env_value API_PG_PASSWORD)" == cloud-runtime-password ]] || fail 'decryption must not replace Cloud business secrets'
  [[ "$(cksum < "$fixture_dir/local-owner.env")" == "$frozen_hash" ]] || fail 'Cloud deployment must preserve encrypted local secrets'
)

printf 'Deploy stack local owner boundary tests passed\n'

(
  stack_case="$fixture_dir/cloud-stack"
  mkdir -p "$stack_case/docker" "$stack_case/bin" "$stack_case/tmp"
  cp "$repo_root/docker/compose.yml" "$stack_case/docker/compose.yml"
  printf 'DOTENV_PUBLIC_KEY="abc"\nPOSTGRES_PASSWORD=encrypted:xyz\n' > "$stack_case/docker/.env"
  for component in api api-migrate auth auth-migrate realtime; do
    image_key="$(printf '%s' "$component" | tr '[:lower:]-' '[:upper:]_')"
    printf 'IDEABOARD_%s_IMAGE=ghcr.io/ericsunsk/ideaboard/ideaboard-%s@sha256:%s\n' "$image_key" "$component" "$digest"
  done > "$stack_case/docker/images.env"
  cat > "$stack_case/bin/dotenvx" <<'BASH'
#!/usr/bin/env bash
printf 'POSTGRES_PASSWORD=local-owner-password\nAPI_PG_PASSWORD=stale-offline-runtime\nPUBLIC_ORIGIN=https://example.com\n'
BASH
  cat > "$stack_case/bin/docker" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
[[ "$POSTGRES_PASSWORD" == local-owner-password && "$API_PG_PASSWORD" == cloud-runtime-password ]]
[[ "$PUBLIC_ORIGIN" == https://app.test.invalid ]]
[[ "$*" == "compose -f $IDEABOARD_STACK_DIR/docker/compose.yml --env-file $IDEABOARD_STACK_DIR/docker/images.env config --quiet" ]]
BASH
  chmod +x "$stack_case/bin/dotenvx" "$stack_case/bin/docker"
  env PATH="$stack_case/bin:$PATH" TMPDIR="$stack_case/tmp" \
    IDEABOARD_STACK_DIR="$stack_case" IDEABOARD_SECRETS_SOURCE=infisical \
    ENV_FILE="$stack_case/docker/.env" IMAGE_ENV_FILE="$stack_case/docker/images.env" \
    DOTENV_PRIVATE_KEY=synthetic-key PUBLIC_ORIGIN=https://app.test.invalid \
    API_PG_PASSWORD=cloud-runtime-password AUTH_PG_PASSWORD=cloud-auth-password \
    AUTH_INTERNAL_KEY=cloud-internal-key APP_SECRETS_ENCRYPTION_KEY=cloud-encryption-key \
    CREDITS_WELCOME_IDENTITY_KEY=cloud-identity-key REWARDS_CODE_HMAC_KEYS=cloud-rewards-keyring \
    RT_TICKET_SECRET=cloud-ticket-key BETTER_AUTH_SECRET=cloud-auth-secret NATS_PASSWORD=cloud-nats-password \
    IMGPROXY_KEY=cloud-imgproxy-key IMGPROXY_SALT=cloud-imgproxy-salt \
    "$deploy_script" config || fail 'Cloud config entrypoint must combine local owner with Cloud business secrets'
  [[ -z "$(ls -A "$stack_case/tmp")" ]] || fail 'config must remove its decrypted temporary file'
)

printf 'Deploy stack Cloud config entrypoint test passed\n'

(
  mkdir -p "$fixture_dir/infisical-bin"
  cat > "$fixture_dir/infisical-bin/infisical" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
while [[ "$1" != -- ]]; do shift; done
shift
export POSTGRES_PASSWORD=synthetic-cloud-password PG_MIGRATION_PASSWORD=synthetic-owner-password
exec "$@"
BASH
  chmod +x "$fixture_dir/infisical-bin/infisical"
  # shellcheck disable=SC2016 # Expand only after owner credential filtering.
  PATH="$fixture_dir/infisical-bin:$PATH" "$repo_root/scripts/infisical-run.sh" --env prod -- \
    bash -c '[[ "$IDEABOARD_SECRETS_SOURCE" == infisical && -z "${POSTGRES_PASSWORD+x}" && -z "${PG_MIGRATION_PASSWORD+x}" ]]' ||
    fail 'Cloud wrapper must identify the injected source and remove owner credentials'

)
printf 'Infisical wrapper source isolation test passed\n'

(
  # Exercise the actual documented production update command through the real
  # wrapper, but stop at a fake CLI: no secrets, Docker actions or deployment.
  mkdir -p "$fixture_dir/recipe-bin"
  cat > "$fixture_dir/recipe-bin/infisical" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
paths=()
while (($# > 0)); do
  case "$1" in
    --path) paths+=("$2"); shift 2 ;;
    --) shift; break ;;
    *) shift ;;
  esac
done
[[ "${#paths[@]}" == 2 && "${paths[0]}" == / && "${paths[1]}" == /ai-proxy ]]
[[ "${ENV_FILE:-}" == /var/lib/ideaboard/owner.env ]]
[[ "$*" == 'env -u POSTGRES_PASSWORD -u PG_MIGRATION_USER -u PG_MIGRATION_PASSWORD scripts/deploy-stack.sh update --manifest release-manifest.json' ]]
BASH
  chmod +x "$fixture_dir/recipe-bin/infisical"
  recipe="$(grep '^ENV_FILE=/var/lib/ideaboard/owner.env scripts/infisical-run.sh --env prod .*scripts/deploy-stack.sh update --manifest release-manifest.json$' "$repo_root/docs/runbooks/deployment.md")"
  [[ -n "$recipe" ]] || fail 'production update recipe is missing'
  (cd "$repo_root" && PATH="$fixture_dir/recipe-bin:$PATH" bash -c "$recipe") ||
    fail 'documented production update must read root and /ai-proxy bindings'
)
printf 'Production update dual-path recipe test passed\n'
