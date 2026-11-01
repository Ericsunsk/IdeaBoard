#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
helper="$root/scripts/deployment-checkout.sh"
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
repo="$fixture/checkout"
git clone --quiet --shared --no-checkout "$root" "$repo"
git -C "$repo" config user.name 'Deployment checkout fixture'
git -C "$repo" config user.email 'deployment-fixture@example.invalid'
git -C "$repo" checkout --quiet --detach HEAD
base=$(git -C "$repo" rev-parse HEAD)

expect_failure() {
  if "$@" >"$fixture/refusal.log" 2>&1; then
    echo "Expected deployment checkout refusal: $*" >&2
    exit 1
  fi
}

expect_failure bash "$helper" "$repo"
expect_failure bash "$helper" --apply "$repo/apps/api"
git -C "$repo" checkout --quiet -b deployment-fixture
expect_failure bash "$helper" --apply "$repo"
git -C "$repo" checkout --quiet --detach "$base"

printf '\nfixture dirty\n' >>"$repo/docker/compose.yml"
expect_failure bash "$helper" --apply "$repo"
git -C "$repo" add docker/compose.yml
expect_failure bash "$helper" --apply "$repo"
git -C "$repo" reset --quiet --hard "$base"

# The next release adds unrelated root and app source. Neither should reappear
# after changing releases, while generated release/secret/image state survives.
cp "$helper" "$repo/scripts/deployment-checkout.sh"
mkdir -p "$repo/apps/web" "$repo/apps/ai-proxy"
printf 'web source\n' >"$repo/apps/web/deployment-fixture.ts"
printf 'proxy source\n' >"$repo/apps/ai-proxy/deployment-fixture.ts"
printf 'unneeded root document\n' >"$repo/DEPLOYMENT-FIXTURE.md"
git -C "$repo" add scripts/deployment-checkout.sh apps/web/deployment-fixture.ts apps/ai-proxy/deployment-fixture.ts DEPLOYMENT-FIXTURE.md
git -C "$repo" -c core.hooksPath=/dev/null commit --quiet -m 'fixture: unrelated source and deployment operation'
next=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" checkout --quiet --detach "$base"
mkdir -p "$repo/docker/releases" "$repo/.cache"
printf 'preserve immutable refs\n' >"$repo/docker/images.env"
printf 'preserve release state\n' >"$repo/docker/releases/current.json"
printf 'preserve local config\n' >"$repo/.cache/deployment.json"

PYTHONDONTWRITEBYTECODE=1 python3 - "$repo" "$fixture/before.json" <<'PY'
import importlib.util
import json
import subprocess
import sys
from pathlib import Path

repo = Path(sys.argv[1])
spec = importlib.util.spec_from_file_location("release_artifacts", repo / "scripts/release-artifacts.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
commit = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
subprocess.run(["git", "-C", str(repo), "tag", "v999.0.0", commit], check=True)
plan = module.make_plan(repo, "Ericsunsk/IdeaBoard", commit, "v999.0.0", "123")
metadata = {name: module.component_metadata(plan, name, "sha256:" + "a" * 64) for name in module.COMPONENTS}
manifest = module.make_manifest(plan, metadata, repo)
Path(sys.argv[2]).write_text(json.dumps(manifest))
PY

bash "$helper" --apply "$repo"
bash "$helper" --apply "$repo"
[[ ! -e $repo/apps && ! -e $repo/README.md && ! -e $repo/AGENTS.md ]]
[[ -f $repo/docker/postgres/init-runtime-roles.sh ]]
[[ -f $repo/docker/compose.yml && -f $repo/docker/compose.npm.yml ]]
[[ -z $(git -C "$repo" status --porcelain) ]]
git -C "$repo" checkout --quiet --detach "$next"
[[ ! -e $repo/apps && ! -e $repo/DEPLOYMENT-FIXTURE.md ]]
bash "$helper" --apply "$repo"
[[ -f $repo/scripts/deployment-checkout.sh ]]
[[ $(cat "$repo/docker/images.env") == 'preserve immutable refs' ]]
[[ $(cat "$repo/docker/releases/current.json") == 'preserve release state' ]]
[[ $(cat "$repo/.cache/deployment.json") == 'preserve local config' ]]
[[ -z $(git -C "$repo" status --porcelain) ]]
printf 'preserve untracked operator file\n' >"$repo/operator-note.txt"
bash "$helper" --apply "$repo"
[[ $(cat "$repo/operator-note.txt") == 'preserve untracked operator file' ]]

PYTHONDONTWRITEBYTECODE=1 python3 - "$repo" "$fixture/before.json" <<'PY'
import importlib.util
import json
import subprocess
import sys
from pathlib import Path

repo = Path(sys.argv[1])
spec = importlib.util.spec_from_file_location("release_artifacts", repo / "scripts/release-artifacts.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
manifest = json.loads(Path(sys.argv[2]).read_text())
module.validate_manifest(manifest, "Ericsunsk/IdeaBoard", root=repo)
commit = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
for name, component in manifest["components"].items():
    assert module.fingerprint(repo, commit, name) == component["input_fingerprint"], name
print("Sparse checkout preserves all five fingerprints and v2 manifest source validation")
PY

# Refusing an incompatible tag must happen before sparse config or runtime data
# changes, including a tracked symlink where a regular bind source is required.
git -C "$repo" rm --quiet scripts/infisical-run.sh
git -C "$repo" -c core.hooksPath=/dev/null commit --quiet -m 'fixture: missing mandatory deployment file'
config_before=$(git -C "$repo" config --get core.sparseCheckout)
patterns_before=$(cat "$repo/.git/info/sparse-checkout")
expect_failure bash "$helper" --apply "$repo"
[[ $(git -C "$repo" config --get core.sparseCheckout) == "$config_before" ]]
[[ $(cat "$repo/.git/info/sparse-checkout") == "$patterns_before" ]]
[[ $(cat "$repo/docker/releases/current.json") == 'preserve release state' ]]
git -C "$repo" checkout --quiet --detach "$next"
git -C "$repo" rm --quiet docker/postgres/init-runtime-roles.sh
mkdir -p "$repo/docker/postgres"
ln -s /etc/passwd "$repo/docker/postgres/init-runtime-roles.sh"
git -C "$repo" add docker/postgres/init-runtime-roles.sh
git -C "$repo" -c core.hooksPath=/dev/null commit --quiet -m 'fixture: unsafe deployment bind source'
expect_failure bash "$helper" --apply "$repo"

echo 'Deployment checkout real-history, state-preservation and refusal fixtures passed'
