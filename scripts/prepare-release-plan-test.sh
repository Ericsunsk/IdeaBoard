#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin"
export FIXTURE="$fixture" GITHUB_REPOSITORY=Ericsunsk/IdeaBoard RELEASE_TAG=v1.2.4 GITHUB_RUN_ID=124
export SOURCE_COMMIT=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
cat > "$fixture/bin/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  'api /repos/Ericsunsk/IdeaBoard/releases/latest') printf '{"tag_name":"v1.2.3"}\n' ;;
  'release download v1.2.3 --repo Ericsunsk/IdeaBoard --pattern release-manifest.json --dir '*)
    cp "$FIXTURE/previous.json" "${@: -1}/release-manifest.json" ;;
  'api /repos/Ericsunsk/IdeaBoard/actions/runs/'*)
    [[ "${TRUST_API_ERROR:-false}" == false ]] || exit 71
    jq -n '{head_repository:{full_name:"Ericsunsk/IdeaBoard"},head_sha:("a"*40),head_branch:"v1.2.3",path:".github/workflows/docker-images.yml",event:"push",status:"completed",conclusion:"success"}' | jq "${TRUST_MUTATION:-.}" ;;
  *) exit 72 ;;
esac
MOCK
cat > "$fixture/bin/python3" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$2" == validate ]]; then [[ "${MANIFEST_INVALID:-false}" == false ]]; exit; fi
[[ "$2" == plan ]]
previous=false
for arg in "$@"; do [[ "$arg" != --previous ]] || previous=true; done
while [[ "$1" != --output ]]; do shift; done
jq -n --argjson previous "$previous" '{used_previous:$previous}' > "$2"
MOCK
chmod +x "$fixture/bin/"*
export PATH="$fixture/bin:$PATH"
jq -n '{schema_version:2,tag:"v1.2.3",commit:("a"*40),workflow:{run_id:123},components:{api:{source_commit:("a"*40),source_tag:"v1.2.3",source_run_id:123}}}' > "$fixture/previous.json"
run_plan() { env TRUST_MUTATION="${TRUST_MUTATION:-.}" TRUST_API_ERROR="${TRUST_API_ERROR:-false}" MANIFEST_INVALID="${MANIFEST_INVALID:-false}" bash "$root/scripts/prepare-release-plan.sh" "$fixture/plan.json" > "$fixture/output" 2>&1 || { cat "$fixture/output" >&2; return 1; }; }
run_plan
jq -e '.used_previous' "$fixture/plan.json" >/dev/null
for mutation in \
  '.head_sha=("c"*40)' '.head_branch="v1.2.2"' '.head_repository.full_name="attacker/IdeaBoard"' \
  '.path=".github/workflows/untrusted.yml"' '.event="workflow_dispatch"' '.conclusion="failure"' '.status="in_progress"'; do
  TRUST_MUTATION="$mutation" run_plan
  jq -e '.used_previous == false' "$fixture/plan.json" >/dev/null || { cat "$fixture/plan.json" "$fixture/output" >&2; exit 1; }
done
TRUST_API_ERROR=true run_plan
jq -e '.used_previous == false' "$fixture/plan.json" >/dev/null || { cat "$fixture/plan.json" "$fixture/output" >&2; exit 1; }
MANIFEST_INVALID=true run_plan
jq -e '.used_previous == false' "$fixture/plan.json" >/dev/null || { cat "$fixture/plan.json" "$fixture/output" >&2; exit 1; }
jq '.schema_version=1' "$fixture/previous.json" > "$fixture/legacy"
mv "$fixture/legacy" "$fixture/previous.json"
run_plan
jq -e '.used_previous == false' "$fixture/plan.json" >/dev/null || { cat "$fixture/plan.json" "$fixture/output" >&2; exit 1; }
printf 'Trusted prior release candidate tests passed\n'
