#!/usr/bin/env bash
# shellcheck disable=SC2329 # Extracted production functions call fixture doubles.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
PYTHONDONTWRITEBYTECODE=1 python3 - "$root" "$fixture" <<'PY'
import importlib.util, json, pathlib, subprocess, sys
root, fixture = map(pathlib.Path, sys.argv[1:])
spec = importlib.util.spec_from_file_location('artifacts', root/'scripts/release-artifacts.py')
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)
repo = fixture/'repo'
repo.mkdir()
def git(*args):
    return subprocess.check_output(['git','-C',str(repo),*args],stderr=subprocess.DEVNULL).decode().strip()
def write(name,text):
    path = repo/name
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(text)
git('init','-q');git('config','user.name','Fixture');git('config','user.email','fixture@example.invalid')
write('.github/workflows/docker-images.yml',(root/'.github/workflows/docker-images.yml').read_text())
for name in ('release-eligibility-check.sh','prepare-release-plan.sh'):
    write('scripts/'+name,(root/'scripts'/name).read_text())
write('.node-version','22\n')
for context in {v[1] for v in engine.COMPONENTS.values()}:
    write(context+'/Dockerfile','FROM scratch AS runtime\nFROM scratch AS migrator\n')
write('scripts/release-artifacts.py',(root/'scripts/release-artifacts.py').read_text())
git('add','.');git('commit','-qm','first');git('tag','v1.0.0')
first = git('rev-parse','HEAD')
plan = engine.make_plan(repo,'Ericsunsk/IdeaBoard',first,'v1.0.0','10')
metadata = {name:engine.component_metadata(plan,name,'sha256:'+'a'*64) for name in engine.COMPONENTS}
previous = engine.make_manifest(plan,metadata,repo)
write('scripts/deploy-only.sh','echo deployment\n')
git('add','.');git('commit','-qm','second');git('tag','v1.0.1')
second = git('rev-parse','HEAD')
plan = engine.make_plan(repo,'Ericsunsk/IdeaBoard',second,'v1.0.1','11',previous)
assert all(x['action']=='reuse' for x in plan['components'].values())
metadata = {name:engine.component_metadata(plan,name) for name in engine.COMPONENTS}
manifest = engine.make_manifest(plan,metadata,repo)
(fixture/'manifest.json').write_text(json.dumps(manifest))
(repo/'deployment-index.json').write_text(json.dumps(dict(tag=manifest['tag'],commit=manifest['commit'])))
(repo/'release-plan.json').write_text(json.dumps(plan))
PY
awk '
  $0 == "      - name: Write image metadata" {found=1;next}
  found && $0 == "        run: |" {block=1;next}
  block && /^          / {sub(/^          /, "");print;next}
  block {exit}
' "$root/.github/workflows/docker-images.yml" > "$fixture/workflow-metadata.sh"
mkdir -p "$fixture/repo/release-assets"
(cd "$fixture/repo" && IMAGE_KEY=api COMPONENT_ACTION=reuse IMAGE_DIGEST="sha256:$(printf 'a%.0s' {1..64})" bash "$fixture/workflow-metadata.sh")
jq -e --slurpfile manifest "$fixture/manifest.json" '. == $manifest[0].components.api' "$fixture/repo/release-assets/api.json" > /dev/null
for function in apply_update_manifest verify_image_signature verify_image_signatures; do
  sed -n "/^${function}() {/,/^}/p" "$root/scripts/deploy-stack.sh" >> "$fixture/functions.sh"
done
# shellcheck source=/dev/null
source "$fixture/functions.sh"
ROOT_DIR="$fixture/repo"
export DEPLOY_SOURCE_MANIFEST_COPY="$fixture/manifest.json"
export ROOT_DIR
export IDEABOARD_DEPLOYMENT_BUNDLE_DIR="$ROOT_DIR"
current_source_commit() { jq -r .commit "$ROOT_DIR/deployment-index.json"; }
current_source_tag() { jq -r .tag "$ROOT_DIR/deployment-index.json"; }
json_string_field() { jq -r --arg key "$1" '.[$key] // empty' "$2"; }
json_image_field() { jq -r --arg key "$1" '.images[$key] // empty' "$2"; }
is_ideaboard_image_ref() { [[ "$1" == "ghcr.io/ericsunsk/ideaboard/ideaboard-$2@sha256:"* ]]; }
die() { echo "$*" >&2; exit 1; }
set_env_value() { export "$1=$2"; }
env_value() { printf '%s' "${!1:-}"; }
cosign() { printf '%s\n' "$*" >> "$fixture/cosign.log"; [[ "${reject_signature:-0}" == 0 ]]; }
apply_update_manifest "$fixture/manifest.json" > /dev/null
COSIGN_VERIFY=false verify_image_signatures > /dev/null
[[ "$(wc -l < "$fixture/cosign.log" | tr -d ' ')" == 5 ]]
[[ "$DEPLOY_COSIGN_GATE" == passed ]]
# Reused source is the old tag, even though the overall release is the new tag.
grep -Fq -- '--certificate-identity https://github.com/Ericsunsk/IdeaBoard/.github/workflows/docker-images.yml@refs/tags/v1.0.0 --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/' "$fixture/cosign.log"
if grep -Fq -- '--certificate-identity-regexp' "$fixture/cosign.log"; then exit 1; fi
# Redeployment of a previous signed bundle uses its own provenance and image refs.
if (IDEABOARD_API_IMAGE="ghcr.io/ericsunsk/ideaboard/ideaboard-api@sha256:$(printf 'b%.0s' {1..64})"; export IDEABOARD_API_IMAGE; verify_image_signatures) > "$fixture/failure" 2>&1; then exit 1; fi
grep -Fq 'provenance does not match deployed api image' "$fixture/failure"
if (reject_signature=1; verify_image_signatures) > "$fixture/failure" 2>&1; then exit 1; fi
grep -Fq 'Image signature verification failed' "$fixture/failure"
# A different source commit cannot be substituted beneath the verified index.
jq '.commit = ("c" * 40)' "$fixture/manifest.json" > "$fixture/stale.json"
if (apply_update_manifest "$fixture/stale.json") > "$fixture/failure" 2>&1; then exit 1; fi
grep -Fq 'bundle source commit does not match release manifest commit' "$fixture/failure"
# Image signatures always require exact provenance; no bypass or broad regex.
if (verify_image_signature "$(jq -r .images.api "$fixture/manifest.json")" '' '') > "$fixture/failure" 2>&1; then exit 1; fi
grep -Fq 'exact component signer identity and trusted issuer are required' "$fixture/failure"
echo 'Release deployment provenance tests passed'
