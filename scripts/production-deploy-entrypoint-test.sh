#!/usr/bin/env bash

set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
entrypoint="${script_dir}/production-deploy-entrypoint.sh"
test_root="$(mktemp -d)"
test_bin="${test_root}/bin"
trap 'rm -rf -- "${test_root}"' EXIT
mkdir -p "${test_bin}"
cat > "${test_bin}/flock" <<'EOF'
#!/usr/bin/env bash
[[ "${FLOCK_SHOULD_FAIL:-0}" != 1 ]]
EOF
cat > "${test_bin}/docker" <<'EOF'
#!/usr/bin/env bash
[[ "$*" == "volume ls --filter name=^ideaboard_postgres-data$ --format {{.Name}}" ]] || exit 97
[[ "${VOLUME_INSPECTION_FAILS:-0}" != 1 ]] || exit 1
[[ "${EXISTING_DATABASE_VOLUME:-0}" != 1 ]] || printf '%s\n' ideaboard_postgres-data
exit 0
EOF
for command in infisical cosign git gh; do
  cat > "${test_bin}/${command}" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${0##*/}" >> "${DEPLOY_CAPTURE_DIR}/unexpected-command"
exit 97
EOF
done
chmod +x "${test_bin}/"*

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
assert_contains() { grep -Fq -- "$2" "$1" || fail "expected $1 to contain $2"; }
assert_no_deploy() {
  [[ ! -e "${capture}/args" && ! -e "${capture}/injection" ]] || fail 'rejected input reached secret injection or deployment'
  [[ -z "$(find "${fixture_root}/tmp" -type f -print -quit)" ]] || fail 'temporary credentials or payload were retained'
}

setup_fixture() {
  fixture_root="${test_root}/$1"
  capture="${fixture_root}/capture"
  stack="${fixture_root}/stack"
  trusted_tree="${fixture_root}/trusted"
  archive="${fixture_root}/deployment-bundle.tar"
  signature="${fixture_root}/deployment-bundle.sigstore.json"
  manifest="${trusted_tree}/release-manifest.json"
  commit=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  tag=v1.2.3
  mkdir -p "${capture}" "${fixture_root}/tmp" "${trusted_tree}/scripts"
  # Deliberate binary NUL tests framing without shell string conversion.
  printf 'signed\000archive\377bytes\n' > "${archive}"
  printf '{"fixture":"signed"}\n' > "${signature}"
  python3 - "${manifest}" "${commit}" "${script_dir}/release-artifacts.py" <<'PYTHON'
import importlib.util, json, pathlib, sys
spec=importlib.util.spec_from_file_location("engine",sys.argv[3]); engine=importlib.util.module_from_spec(spec); spec.loader.exec_module(engine)
names = ['api', 'api-migrate', 'auth', 'auth-migrate', 'realtime']
repo='Ericsunsk/IdeaBoard'; ref='refs/tags/v1.2.3'
components={}
for n in names:
    item=engine.base_component(repo,sys.argv[2],'v1.2.3','10',n,'sha256:'+'a'*64,ref)
    image='ghcr.io/ericsunsk/ideaboard/ideaboard-'+n+'@sha256:'+'a'*64
    item.update(name=n,ref=image,image=image,digest='sha256:'+'a'*64)
    components[n]=item
value=dict(schema_version=2,repository=repo,tag='v1.2.3',commit=sys.argv[2],ref=ref,release_kind='production',workflow=dict(name='docker-images.yml',run_id='10'),images={n:item['image'] for n,item in components.items()},components=components,cosign=dict(certificate_identity=engine.identity(repo,ref),certificate_oidc_issuer=engine.ISSUER))
pathlib.Path(sys.argv[1]).write_text(json.dumps(value))
PYTHON
  cp "${script_dir}/release-artifacts.py" "${trusted_tree}/scripts/release-artifacts.py"
  cat > "${trusted_tree}/scripts/deploy-stack.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ -z "${INFISICAL_TOKEN:-}" && "${IDEABOARD_SECRETS_SOURCE:-}" == infisical ]] || exit 96
printf '%s\n' "$*" > "${DEPLOY_CAPTURE_DIR}/args"
printf '%s\n' "$3" > "${DEPLOY_CAPTURE_DIR}/manifest-path"
cp "$3" "${DEPLOY_CAPTURE_DIR}/manifest-copy"
printf '%s\n' "${IDEABOARD_STACK_DIR}" > "${DEPLOY_CAPTURE_DIR}/stack-dir"
printf '%s\n' "${ENV_FILE}" > "${DEPLOY_CAPTURE_DIR}/env-file"
printf '%s\n' "${COMPOSE_PROJECT_NAME}" > "${DEPLOY_CAPTURE_DIR}/compose-project"
printf '%s\n' "${IDEABOARD_COMPOSE_OVERRIDE_FILE:-}" "${NPM_NETWORK_NAME:-}" > "${DEPLOY_CAPTURE_DIR}/topology"
printf '%s\n' "${IMAGE_ENV_FILE}" "${RELEASE_STATE_DIR}" "${BACKUP_DIR}" > "${DEPLOY_CAPTURE_DIR}/state-paths"
printf '%s\n' "${IDEABOARD_DEPLOYMENT_BUNDLE_DIR}" "${IDEABOARD_DEPLOYMENT_BUNDLE_DIGEST}" "${IDEABOARD_DEPLOYMENT_BUNDLE_ARCHIVE}" "${IDEABOARD_DEPLOYMENT_BUNDLE_SIGNATURE}" > "${DEPLOY_CAPTURE_DIR}/bundle"
printf 'RAW-DEPLOY-SECRET-MARKER\n' >&2
if [[ "${SKIP_DEPLOY_RESULT:-0}" != 1 ]]; then
  if [[ "${DEPLOY_SHOULD_FAIL:-0}" == 1 ]]; then
    printf '%s\n' '{"result":"failure","stage":"smoke","recovery_hint":"inspect_release_state_and_service_logs_on_vps","secret":"RAW-RESULT-SECRET-MARKER"}' > "${IDEABOARD_DEPLOY_RESULT_FILE}"
  else
    printf '%s\n' '{"result":"success","stage":"complete","recovery_hint":"none","secret":"RAW-RESULT-SECRET-MARKER"}' > "${IDEABOARD_DEPLOY_RESULT_FILE}"
  fi
fi
[[ "${DEPLOY_SHOULD_FAIL:-0}" != 1 ]] || exit 23
EOF
  cat > "${trusted_tree}/scripts/infisical-run.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${INFISICAL_TOKEN:-}" == st.SYNTHETIC-INFISICAL-TOKEN ]] || exit 95
[[ "$PWD" == "$IDEABOARD_STACK_DIR" ]] || exit 93
[[ "$1 $2 $3 $4 $5 $6 $7" == '--env prod --path / --path /ai-proxy --' ]] || exit 94
export IDEABOARD_COMPOSE_OVERRIDE_FILE="${CLOUD_COMPOSE_OVERRIDE:-${IDEABOARD_COMPOSE_OVERRIDE_FILE:-}}"
export NPM_NETWORK_NAME="${CLOUD_NPM_NETWORK:-${NPM_NETWORK_NAME:-}}"
# Model Cloud collisions with host-owned state and security settings.
export IDEABOARD_SECRETS_SOURCE=cloud-untrusted IDEABOARD_STACK_DIR=/cloud-untrusted IMAGE_ENV_FILE=/cloud-untrusted
export RELEASE_STATE_DIR=/cloud-untrusted BACKUP_DIR=/cloud-untrusted ENV_FILE=/cloud-untrusted
export IDEABOARD_DEPLOYMENT_BUNDLE_DIR=/cloud-untrusted IDEABOARD_DEPLOYMENT_BUNDLE_DIGEST=/cloud-untrusted
export IDEABOARD_DEPLOYMENT_BUNDLE_ARCHIVE=/cloud-untrusted IDEABOARD_DEPLOYMENT_BUNDLE_SIGNATURE=/cloud-untrusted
export IDEABOARD_DEPLOY_RESULT_FILE=/cloud-untrusted COMPOSE_PROJECT_NAME=cloud-untrusted
printf 'prod root+ai-proxy injected\n' > "${DEPLOY_CAPTURE_DIR}/injection"
shift 7
exec "$@"
EOF
  chmod +x "${trusted_tree}/scripts/"*.sh
  cat > "${fixture_root}/verifier.py" <<'PYTHON'
import argparse, os, pathlib, shutil, sys
p = argparse.ArgumentParser()
p.add_argument('command', choices=['verify'])
for key in ['archive', 'signature', 'repository', 'tag', 'commit', 'output-dir']:
    p.add_argument('--'+key, required=True)
a = p.parse_args()
root = pathlib.Path(os.environ['TRUSTED_TREE'])
if os.environ.get('SIGNATURE_FAIL') == '1':
    sys.exit(1)
if pathlib.Path(a.archive).read_bytes() != pathlib.Path(os.environ['EXPECTED_ARCHIVE']).read_bytes():
    sys.exit(1)
if pathlib.Path(a.signature).read_bytes() != b'{"fixture":"signed"}\n':
    sys.exit(1)
if (a.repository, a.tag, a.commit) != ('Ericsunsk/IdeaBoard', os.environ['EXPECTED_TAG'], os.environ['EXPECTED_COMMIT']):
    sys.exit(1)
shutil.copytree(root, a.output_dir)
PYTHON
  chmod 600 "${fixture_root}/verifier.py"
}

run_command() {
  env PATH="${test_bin}:${PATH}" DEPLOY_CAPTURE_DIR="${capture}" TRUSTED_TREE="${trusted_tree}" \
    EXPECTED_ARCHIVE="${archive}" EXPECTED_TAG="${tag}" EXPECTED_COMMIT="${commit}" \
    IDEABOARD_PRODUCTION_STACK_DIR="${stack}" \
    IDEABOARD_PRODUCTION_LOCK_FILE="${fixture_root}/deploy.lock" \
    IDEABOARD_PRODUCTION_TMP_DIR="${fixture_root}/tmp" \
    IDEABOARD_PRODUCTION_OWNER_ENV_FILE="${fixture_root}/state/owner.env" \
    IDEABOARD_PRODUCTION_CONFIG_FILE="${fixture_root}/deployment.json" \
    IDEABOARD_PRODUCTION_BUNDLE_COMMAND="${fixture_root}/verifier.py" \
    "$@"
}
run_raw() { run_command bash "${entrypoint}" "$@"; }
write_transport() {
  python3 - "${archive}" "${signature}" "${tag}" "${commit}" <<'PYTHON'
import json, pathlib, sys
archive, signature = (pathlib.Path(p).read_bytes() for p in sys.argv[1:3])
header = dict(tag=sys.argv[3], commit=sys.argv[4], archive_bytes=len(archive), signature_bytes=len(signature))
sys.stdout.buffer.write(('IDEABOARD_DEPLOY_V2\nst.SYNTHETIC-INFISICAL-TOKEN\n'+json.dumps(header)+'\n').encode()+archive+signature)
PYTHON
}
run_entrypoint() { write_transport | run_raw "$@"; }

setup_fixture success
output="$(CLOUD_COMPOSE_OVERRIDE=cloud.yml CLOUD_NPM_NETWORK=wrong run_entrypoint 2>&1)"
[[ "$output" == '{"result":"success","stage":"complete","recovery_hint":"none"}' ]] || fail "valid bundle failed: $output"
assert_contains "${capture}/args" 'deploy --manifest '
cmp -s "${manifest}" "${capture}/manifest-copy" || fail 'signed manifest changed'
deployed_manifest_path="$(cat "${capture}/manifest-path")"
[[ -f "$deployed_manifest_path" ]] || fail 'signed bundle was not retained for rollback'
assert_contains "${capture}/env-file" "${fixture_root}/state/owner.env"
assert_contains "${capture}/compose-project" ideaboard
[[ "$(tr -d '\n' < "${capture}/topology")" == "" ]] || fail 'Cloud topology was accepted without host configuration'
assert_contains "${capture}/state-paths" "${stack}/docker/images.env"
assert_contains "${capture}/state-paths" "${stack}/docker/releases"
assert_contains "${capture}/bundle" sha256:
[[ ! -e "${capture}/unexpected-command" && ! -e "${stack}/.git" && ! -e "${stack}/apps" ]] || fail 'runtime invoked source tools or retained repository'
if grep -rFq 'SYNTHETIC-INFISICAL-TOKEN' "${capture}" "${fixture_root}/tmp"; then fail 'token retained'; fi

for case_name in signature-failure tampered-signature tag-binding commit-binding; do
  setup_fixture "$case_name"
  case "$case_name" in
    signature-failure) export SIGNATURE_FAIL=1 ;;
    tampered-signature) printf 'unsigned' > "${signature}" ;;
    tag-binding) tag=v1.2.4 ;;
    commit-binding) commit=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb ;;
  esac
  if [[ "$case_name" == tag-binding || "$case_name" == commit-binding ]]; then
    # Change transport identity while keeping verifier expectations pinned.
    write_transport > "${fixture_root}/transport"
    tag=v1.2.3; commit=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    if run_raw < "${fixture_root}/transport" >/dev/null 2>&1; then fail "$case_name accepted"; fi
  elif run_entrypoint >/dev/null 2>&1; then fail "$case_name accepted"; fi
  unset SIGNATURE_FAIL
  assert_no_deploy
done

for kind in token-empty token-space token-cr token-too-long v1 unsigned header-too-long duplicate bool-size oversized-archive oversized-signature negative-size truncated trailing invalid-tag invalid-commit; do
  setup_fixture "$kind"
  write_transport > "${fixture_root}/transport"
  python3 - "${fixture_root}/transport" "$kind" <<'PYTHON'
import json, pathlib, sys
p=pathlib.Path(sys.argv[1]); kind=sys.argv[2]
protocol, token, raw, body=p.read_bytes().split(b'\n',3)
h=json.loads(raw)
if kind=='token-empty': token=b''
elif kind=='token-space': token=b'invalid token'
elif kind=='token-cr': token=b'token\r'
elif kind=='token-too-long': token=b'x'*8193
elif kind=='v1': protocol=b'IDEABOARD_DEPLOY_V1'
elif kind=='unsigned': protocol=b'{"tag":"v1.2.3"}'
elif kind=='header-too-long': raw=b'x'*4097
elif kind=='duplicate': raw=raw[:-1]+b',"tag":"v1.2.3"}'
elif kind=='bool-size': h['archive_bytes']=True
elif kind=='oversized-archive': h['archive_bytes']=67108865
elif kind=='oversized-signature': h['signature_bytes']=1048577
elif kind=='negative-size': h['archive_bytes']=-1
elif kind=='truncated': body=body[:-1]
elif kind=='trailing': body+=b'TRAILING-SECRET-MARKER'
elif kind=='invalid-tag': h['tag']='main'
elif kind=='invalid-commit': h['commit']='a'*12
if kind not in ('duplicate','header-too-long'): raw=json.dumps(h).encode()
p.write_bytes(b'\n'.join([protocol,token,raw,body]))
PYTHON
  if output="$(run_raw < "${fixture_root}/transport" 2>&1)"; then fail "$kind accepted"; fi
  [[ "$output" != *SYNTHETIC-INFISICAL-TOKEN* && "$output" != *TRAILING-SECRET-MARKER* ]] || fail 'untrusted input printed'
  assert_no_deploy
done

for kind in wrong-repository invalid-digest invalid-provenance; do
  setup_fixture "$kind"
  case "$kind" in
    wrong-repository) filter='.repository="attacker/IdeaBoard"' ;;
    invalid-digest) filter='.images.api="ghcr.io/ericsunsk/ideaboard/ideaboard-api:latest"' ;;
    invalid-provenance) filter='.components.api.input_fingerprint="invalid"' ;;
  esac
  jq "$filter" "${manifest}" > "${manifest}.new"
  mv "${manifest}.new" "${manifest}"
  if run_entrypoint >/dev/null 2>&1; then fail "$kind accepted"; fi
  assert_no_deploy
done

setup_fixture existing-owner
if EXISTING_DATABASE_VOLUME=1 run_entrypoint >/dev/null 2>&1; then fail 'existing DB accepted missing owner'; fi
assert_no_deploy
mkdir -p "${fixture_root}/state"
printf 'POSTGRES_PASSWORD=existing-owner-password\n' > "${fixture_root}/state/owner.env"
EXISTING_DATABASE_VOLUME=1 run_entrypoint >/dev/null
assert_contains "${fixture_root}/state/owner.env" POSTGRES_PASSWORD=existing-owner-password
setup_fixture volume-failure
if VOLUME_INSPECTION_FAILS=1 run_entrypoint >/dev/null 2>&1; then fail 'volume inspection failure accepted'; fi
assert_no_deploy
setup_fixture lock-failure
if FLOCK_SHOULD_FAIL=1 run_entrypoint >/dev/null 2>&1; then fail 'lock contention accepted'; fi
assert_no_deploy
setup_fixture caller-command
if run_entrypoint unexpected >/dev/null 2>&1; then fail 'caller args accepted'; fi
if SSH_ORIGINAL_COMMAND='cat /etc/passwd' run_entrypoint >/dev/null 2>&1; then fail 'SSH command accepted'; fi
assert_no_deploy

setup_fixture deploy-failure
if output="$(DEPLOY_SHOULD_FAIL=1 run_entrypoint 2>&1)"; then fail 'deploy failure returned success'; fi
[[ "$output" == '{"result":"failure","stage":"smoke","recovery_hint":"inspect_release_state_and_service_logs_on_vps"}' ]] || fail 'result was not sanitized'
[[ -f "$(cat "${capture}/manifest-path")" ]] || fail 'verified failed release artifact not retained'
[[ -z "$(find "${fixture_root}/tmp" -type f -print -quit)" ]] || fail 'temporary result retained'
setup_fixture missing-result
if output="$(SKIP_DEPLOY_RESULT=1 run_entrypoint 2>&1)"; then fail 'missing result returned success'; fi
[[ "$output" == '{"result":"failure","stage":"unknown","recovery_hint":"inspect_release_state_and_service_logs_on_vps"}' ]] || fail 'missing result not fail-closed'

setup_fixture topology
printf '{"npm_network":"npm-nw"}\n' > "${fixture_root}/deployment.json"
chmod 600 "${fixture_root}/deployment.json"
CLOUD_COMPOSE_OVERRIDE=cloud.yml CLOUD_NPM_NETWORK=wrong run_entrypoint >/dev/null
assert_contains "${capture}/topology" docker/compose.npm.yml
assert_contains "${capture}/topology" npm-nw
for kind in malformed unknown duplicate unsafe-permissions symlink; do
  setup_fixture "config-$kind"
  case "$kind" in
    malformed) printf '{' > "${fixture_root}/deployment.json" ;;
    unknown) printf '{"npm_network":"npm-nw","other":true}' > "${fixture_root}/deployment.json" ;;
    duplicate) printf '{"npm_network":"npm-nw","npm_network":"evil"}' > "${fixture_root}/deployment.json" ;;
    unsafe-permissions) printf '{"npm_network":"npm-nw"}' > "${fixture_root}/deployment.json"; chmod 666 "${fixture_root}/deployment.json" ;;
    symlink) printf '{"npm_network":"npm-nw"}' > "${fixture_root}/config"; ln -s config "${fixture_root}/deployment.json" ;;
  esac
  if run_raw </dev/null >/dev/null 2>&1; then fail "$kind host config accepted"; fi
  assert_no_deploy
done
setup_fixture bootstrap
run_command env INFISICAL_TOKEN=st.SYNTHETIC-INFISICAL-TOKEN IDEABOARD_PRODUCTION_ENTRYPOINT="${entrypoint}" \
  bash "${script_dir}/bootstrap-vps.sh" --ref "$tag" --manifest "$manifest" --bundle "$archive" --signature "$signature" >/dev/null
[[ -f "${capture}/args" ]] || fail 'local signed bootstrap did not reach trusted entrypoint'
rm "${capture}/args" "${capture}/injection"
if run_command env INFISICAL_TOKEN=st.SYNTHETIC-INFISICAL-TOKEN IDEABOARD_PRODUCTION_ENTRYPOINT="${entrypoint}" \
  bash "${script_dir}/bootstrap-vps.sh" --ref v9.9.9 --manifest "$manifest" --bundle "$archive" --signature "$signature" >/dev/null 2>&1; then
  fail 'bootstrap accepted tag differing from local manifest'
fi
assert_no_deploy

# Exercise the actual sender and receiver together. Only SSH/timeout are
# replaced; both production binaries handle the complete signed byte stream.
cat > "${test_bin}/timeout" <<'EOF'
#!/usr/bin/env bash
[[ "$1" == --foreground && "$2" == 25m ]] || exit 97
shift 2
exec "$@"
EOF
cat > "${test_bin}/ssh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "${DEPLOY_CAPTURE_DIR}/ssh-args"
count=0
[[ ! -f "${DEPLOY_CAPTURE_DIR}/attempts" ]] || read -r count < "${DEPLOY_CAPTURE_DIR}/attempts"
count=$((count+1))
printf '%s\n' "$count" > "${DEPLOY_CAPTURE_DIR}/attempts"
# Drain the stream for simulated failures so sender-side SIGPIPE cannot change
# the intended SSH exit status in the fixture.
case "${TRANSPORT_TEST_MODE:-success}" in
  safe-retry)
    if (( count == 1 )); then
      cat >/dev/null
      printf '%s\n' '{"result":"failure","stage":"preflight","recovery_hint":"inspect_release_state_and_service_logs_on_vps"}'
      exit 23
    fi
    ;;
  migration)
    cat >/dev/null
    printf '%s\n' '{"result":"failure","stage":"api-migration","recovery_hint":"inspect_release_state_and_backups_before_manual_recovery"}'
    exit 23
    ;;
  disconnect) cat >/dev/null; exit 255 ;;
  timeout) cat >/dev/null; exit 124 ;;
esac
exec bash "$ENTRYPOINT_TEST"
EOF
chmod +x "${test_bin}/timeout" "${test_bin}/ssh"
run_transport() {
  mkdir -p "${fixture_root}/runner"
  : > "${fixture_root}/ssh-key"
  : > "${fixture_root}/known-hosts"
  run_command env INFISICAL_TOKEN=st.SYNTHETIC-INFISICAL-TOKEN ENTRYPOINT_TEST="${entrypoint}" \
    RUNNER_TEMP="${fixture_root}/runner" VPS_HOST=vps.example.invalid VPS_SSH_PORT=5522 VPS_DEPLOY_USER=root \
    SSH_KEY_FILE="${fixture_root}/ssh-key" SSH_KNOWN_HOSTS_FILE="${fixture_root}/known-hosts" \
    bash "${script_dir}/run-production-deploy.sh" "$manifest" "$archive" "$signature"
}
for mode in success safe-retry migration disconnect timeout; do
  setup_fixture "transport-$mode"
  if output="$(TRANSPORT_TEST_MODE="$mode" run_transport 2>&1)"; then
    [[ "$mode" == success || "$mode" == safe-retry ]] || fail "$mode transport failure returned success"
    [[ -e "${capture}/args" ]] || fail 'real sender/receiver did not deploy signed bundle'
  else
    [[ "$mode" != success && "$mode" != safe-retry ]] || fail "valid transport failed: $output"
    assert_no_deploy
  fi
  expected_attempts=1
  [[ "$mode" != safe-retry ]] || expected_attempts=2
  [[ "$(cat "${capture}/attempts")" == "$expected_attempts" ]] || fail "$mode violated migration-safe retry policy"
  assert_contains "${capture}/ssh-args" StrictHostKeyChecking=yes
  assert_contains "${capture}/ssh-args" PasswordAuthentication=no
  [[ "$output" != *SYNTHETIC-INFISICAL-TOKEN* && "$output" != *RAW-DEPLOY-SECRET-MARKER* ]] || fail 'transport exposed credential or raw deployment output'
  [[ ! -e "${fixture_root}/runner/production-deploy-result.raw" && ! -e "${fixture_root}/runner/production-deploy-error.raw" ]] || fail 'raw SSH output retained'
done

setup_fixture unsafe-verifier
chmod 666 "${fixture_root}/verifier.py"
if run_entrypoint >/dev/null 2>&1; then fail 'writable verifier accepted'; fi
assert_no_deploy

setup_fixture retained-state
mkdir -p "${stack}/docker/releases/history"
printf 'keep state\n' > "${stack}/docker/releases/history/retained.json"
printf 'keep image refs\n' > "${stack}/docker/images.env"
run_entrypoint >/dev/null
first_manifest="$(cat "${capture}/manifest-path")"
# Second release uses a different verified input tree and identity.
tag=v1.2.4; commit=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
jq --arg tag "$tag" --arg commit "$commit" '.tag=$tag|.commit=$commit|.ref=("refs/tags/"+$tag)|.cosign.certificate_identity=("https://github.com/Ericsunsk/IdeaBoard/.github/workflows/docker-images.yml@refs/tags/"+$tag)' "${manifest}" > "${manifest}.new"
mv "${manifest}.new" "${manifest}"
run_entrypoint >/dev/null
[[ -f "$first_manifest" && "$first_manifest" != "$(cat "${capture}/manifest-path")" ]] || fail 'previous verified bundle overwritten'
assert_contains "${stack}/docker/releases/history/retained.json" 'keep state'
assert_contains "${stack}/docker/images.env" 'keep image refs'
[[ ! -e "${capture}/unexpected-command" && ! -e "${stack}/.git" && ! -e "${stack}/apps" ]] || fail 'repository tools or sources used'
printf 'Production deploy entrypoint contract tests passed\n'
