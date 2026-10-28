#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
action="${root}/.github/actions/deploy-production/action.yml"
fixture="$(mktemp -d)"
trap 'rm -rf -- "${fixture}"' EXIT
mkdir -p "${fixture}/steps" "${fixture}/bin"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

# Run the actual composite shell blocks; no GitHub, network, or VPS access.
extract_step() {
  local name="$1" target="$2"
  awk -v step="${name}" '
    $0 == "    - name: " step { found = 1; next }
    found && /^      run: / {
      if ($0 == "      run: |") { block = 1; next }
      sub(/^      run: /, ""); print; exit
    }
    block && /^[[:space:]]*$/ { print ""; next }
    block && /^        / { sub(/^        /, ""); print; next }
    block { exit }
  ' "${action}" > "${target}"
  [[ -s "${target}" ]] || fail "missing composite step: ${name}"
}
extract_step 'Validate immutable release manifest' "${fixture}/steps/validate"
extract_step 'Configure restricted SSH transport' "${fixture}/steps/configure"
extract_step 'Stream manifest to Production deploy entrypoint' "${fixture}/steps/deploy"
extract_step 'Remove SSH credentials' "${fixture}/steps/cleanup"
extract_step 'Write Production deployment summary' "${fixture}/steps/summary"

cat > "${fixture}/bin/ssh-keygen" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" > "${RUNNER_TEMP}/keygen-args"
[[ "${TEST_UNKNOWN_HOST:-false}" == false ]]
MOCK
cat > "${fixture}/bin/timeout" <<'MOCK'
#!/usr/bin/env bash
[[ "$1" == --foreground && "$2" == 25m ]] || exit 97
shift 2
exec "$@"
MOCK
cat > "${fixture}/bin/ssh" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" > "${RUNNER_TEMP}/ssh-args"
cat > "${RUNNER_TEMP}/sent-manifest"
printf 'RAW-SSH-SECRET-MARKER\n' >&2
if [[ "${TEST_DEPLOY_FAIL:-false}" == true ]]; then
  printf '%s\n' '{"result":"failure","stage":"api-migration","recovery_hint":"inspect_release_state_and_backups_before_manual_recovery"}'
  exit 23
fi
printf '%s\n' '{"result":"success","stage":"complete","recovery_hint":"none","secret":"RAW-RESULT-SECRET-MARKER"}'
MOCK
chmod +x "${fixture}/bin/"*
export PATH="${fixture}/bin:${PATH}"
export PREREQUISITE_STATUS=success
export GITHUB_REPOSITORY='Ericsunsk/IdeaBoard' RELEASE_TAG='v1.2.3'
export SOURCE_COMMIT='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
export VPS_HOST='vps.example.com' VPS_SSH_PORT='5522' VPS_DEPLOY_USER='root'
export VPS_DEPLOY_PRIVATE_KEY='FAKE-DEPLOY-PRIVATE-KEY' VPS_KNOWN_HOSTS='FAKE-PINNED-HOST-KEY'
export RELEASE_MANIFEST="${fixture}/release-manifest.json"
export RUNNER_TEMP="${fixture}/runner" GITHUB_STEP_SUMMARY="${fixture}/summary"

write_manifest() {
  jq -n --arg commit "${SOURCE_COMMIT}" \
    '{repository: "Ericsunsk/IdeaBoard", tag: "v1.2.3", ref: "refs/tags/v1.2.3", commit: $commit,
      images: (["api", "api-migrate", "auth", "auth-migrate", "realtime"] |
        map({key: ., value: ("ghcr.io/ericsunsk/ideaboard/ideaboard-" + . + "@sha256:" + ("b" * 64))}) | from_entries)}' > "${RELEASE_MANIFEST}"
}

run_action() {
  local status=0
  rm -rf -- "${RUNNER_TEMP}"
  mkdir -p "${RUNNER_TEMP}"
  : > "${GITHUB_STEP_SUMMARY}"
  bash "${fixture}/steps/validate" > "${fixture}/output" 2>&1 || status=$?
  if (( status == 0 )); then
    bash "${fixture}/steps/configure" >> "${fixture}/output" 2>&1 || status=$?
  fi
  if (( status == 0 )); then
    (cd "${root}" && bash "${fixture}/steps/deploy") >> "${fixture}/output" 2>&1 || status=$?
  fi
  bash "${fixture}/steps/cleanup"
  [[ ! -e "${RUNNER_TEMP}/ideaboard-production-ssh" ]] || fail 'SSH credentials were not cleaned up'
  RESULT=failure
  (( status == 0 )) && RESULT=success
  export RESULT
  bash "${fixture}/steps/summary"
  if grep -Eq 'FAKE-DEPLOY-PRIVATE-KEY|FAKE-PINNED-HOST-KEY|RAW-SSH-SECRET-MARKER|RAW-RESULT-SECRET-MARKER' "${fixture}/output" "${GITHUB_STEP_SUMMARY}"; then
    fail 'credentials or raw remote logs leaked'
  fi
  return "${status}"
}

write_manifest
run_action || { cat "${fixture}/output" >&2; fail 'valid new release failed'; }
cmp -s "${RELEASE_MANIFEST}" "${RUNNER_TEMP}/sent-manifest" || fail 'transport did not receive exact manifest'
for option in 'StrictHostKeyChecking=yes' 'PasswordAuthentication=no' 'BatchMode=yes' 'root@vps.example.com'; do
  grep -Fq -- "${option}" "${RUNNER_TEMP}/ssh-args" || fail "missing transport option: ${option}"
done
grep -Fq -- '-F [vps.example.com]:5522' "${RUNNER_TEMP}/keygen-args" || fail 'nonstandard port host key was not pinned'
grep -Fq -- "${SOURCE_COMMIT}" "${GITHUB_STEP_SUMMARY}" || fail 'summary missing source commit'
# shellcheck disable=SC2016 # Backticks are literal Markdown code spans in the summary.
grep -Fq -- '| Failed stage | `none` |' "${GITHUB_STEP_SUMMARY}" || fail 'success summary missing stage'

SOURCE_COMMIT='' run_action || fail 'valid existing release failed without a preflight source commit'

for mutation in \
  '.tag = "v1.2.4"' \
  '.ref = "refs/heads/main"' \
  '.repository = "attacker/IdeaBoard"' \
  '.commit = "cccccccccccccccccccccccccccccccccccccccc"' \
  '.commit = "short-sha"' \
  '.images.api = "ghcr.io/attacker/ideaboard/ideaboard-api@sha256:" + ("b" * 64)' \
  '.images.api = "ghcr.io/ericsunsk/ideaboard/ideaboard-api:latest"' \
  '.images.api |= sub("@sha256:"; "@sha256:garbage@sha256:")' \
  'del(.images.realtime)'; do
  write_manifest
  jq "${mutation}" "${RELEASE_MANIFEST}" > "${fixture}/changed"
  mv "${fixture}/changed" "${RELEASE_MANIFEST}"
  if run_action; then fail "invalid manifest accepted: ${mutation}"; fi
  [[ ! -e "${RUNNER_TEMP}/keygen-args" && ! -e "${RUNNER_TEMP}/ssh-args" ]] || fail 'invalid manifest reached SSH setup'
done

write_manifest
for prerequisite_status in failure cancelled skipped; do
  if PREREQUISITE_STATUS="${prerequisite_status}" run_action; then fail 'failed prerequisite accepted'; fi
  [[ ! -e "${RUNNER_TEMP}/ssh-args" ]] || fail 'failed prerequisite reached transport'
  grep -Fq -- 'not-started' "${GITHUB_STEP_SUMMARY}" || fail 'prerequisite failure omitted summary'
done
if SOURCE_COMMIT=short-sha run_action; then fail 'invalid preflight commit accepted'; fi
for invalid_port in 0 65536 '22;echo injected'; do
  if VPS_SSH_PORT="${invalid_port}" run_action; then fail 'invalid SSH port accepted'; fi
  [[ ! -e "${RUNNER_TEMP}/ssh-args" ]] || fail 'invalid SSH port reached transport'
done
if VPS_DEPLOY_PRIVATE_KEY='' run_action; then fail 'missing private key accepted'; fi
if TEST_UNKNOWN_HOST=true run_action; then fail 'untrusted host key accepted'; fi
[[ ! -e "${RUNNER_TEMP}/ssh-args" ]] || fail 'untrusted host reached transport'
if TEST_DEPLOY_FAIL=true run_action; then fail 'migration failure accepted'; fi
# shellcheck disable=SC2016 # Backticks are literal Markdown code spans in the summary.
grep -Fq -- '| Failed stage | `api-migration` |' "${GITHUB_STEP_SUMMARY}" || fail 'failure summary missing migration stage'
grep -Fq -- 'inspect_release_state_and_backups_before_manual_recovery' "${GITHUB_STEP_SUMMARY}" || fail 'failure summary missing recovery hint'

printf 'Production deployment action fixture tests passed\n'
