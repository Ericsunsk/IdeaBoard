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

printf 'Deploy stack manifest contract tests passed\n'
