#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin" "$fixture/repo"
git init -q "$fixture/repo"
git -C "$fixture/repo" -c user.name=Fixture -c user.email=fixture@example.invalid commit -qm fixture --allow-empty
commit="$(git -C "$fixture/repo" rev-parse HEAD)"
git -C "$fixture/repo" update-ref refs/remotes/origin/main "$commit"
git -C "$fixture/repo" tag v1.2.3
REAL_GIT="$(command -v git)"
export REAL_GIT FIXTURE_COMMIT="$commit" GITHUB_REPOSITORY=Ericsunsk/IdeaBoard REQUESTED_REF=v1.2.3
cat > "$fixture/bin/git" <<'MOCK'
#!/usr/bin/env bash
if [[ "$1" == -c ]]; then
  [[ "${FETCH_FAIL:-false}" == false ]] || exit 74
  [[ "$*" == "-c credential.helper= -c credential.helper=!gh auth git-credential fetch origin main --no-tags" ]] || exit 71
  exit 0
fi
exec "$REAL_GIT" "$@"
MOCK
cat > "$fixture/bin/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ "${API_FAIL:-false}" == false ]] || exit 75
case "$*" in
  *'/actions/workflows/ci.yml/runs?'*)
    [[ "$*" == *"head_sha=$FIXTURE_COMMIT&branch=main&event=push&"* ]] || exit 72
    if [[ "${STALE_STATUS_INDEX:-false}" == true && "$*" == *'&status='* ]]; then
      printf '[{"workflow_runs":[]}]\n'
      exit 0
    fi
    jq -n --arg commit "$FIXTURE_COMMIT" '[{workflow_runs:[{id:123,head_sha:$commit,head_branch:"main",event:"push",head_repository:{full_name:"Ericsunsk/IdeaBoard"},path:".github/workflows/ci.yml",status:"completed",conclusion:"success"}]}]' | jq "${RUN_MUTATION:-.}"
    ;;
  *'/actions/runs/123/jobs?filter=latest&per_page=100'*)
    jq -n --arg commit "$FIXTURE_COMMIT" '[{jobs:[{name:"CI Gate",head_sha:$commit,status:"completed",conclusion:"success"}]}]' | jq "${JOB_MUTATION:-.}"
    ;;
  *) exit 73 ;;
esac
MOCK
chmod +x "$fixture/bin/"*
export PATH="$fixture/bin:$PATH"
run_check() { (cd "$fixture/repo" && env REQUESTED_REF="$REQUESTED_REF" RUN_MUTATION="${RUN_MUTATION:-.}" JOB_MUTATION="${JOB_MUTATION:-.}" bash "$root/scripts/release-eligibility-check.sh") > "$fixture/output" 2>&1; }
run_check || { cat "$fixture/output" >&2; exit 1; }
grep -Fxq "source_commit=$commit" "$fixture/output"
grep -Fxq 'trusted_ci_run_id=123' "$fixture/output"
# GitHub's server-side status index can omit a completed successful run.
STALE_STATUS_INDEX=true run_check || { cat "$fixture/output" >&2; exit 1; }
for mutation in \
  '.[0].workflow_runs=[]' \
  '.[0].workflow_runs[0].head_sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"' \
  '.[0].workflow_runs[0].head_branch="feature"' \
  '.[0].workflow_runs[0].event="pull_request"' \
  '.[0].workflow_runs[0].head_repository.full_name="attacker/IdeaBoard"' \
  '.[0].workflow_runs[0].path=".github/workflows/attacker.yml"' \
  '.[0].workflow_runs[0].status="in_progress"' \
  '.[0].workflow_runs[0].conclusion="failure"' \
  '.[0].workflow_runs[0].id="123/other"'; do
  if RUN_MUTATION="$mutation" run_check; then echo "accepted untrusted run: $mutation" >&2; exit 1; fi
done
for mutation in \
  '.[0].jobs=[]' \
  '.[0].jobs[0].name="Forged Gate"' \
  '.[0].jobs[0].head_sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"' \
  '.[0].jobs[0].conclusion="skipped"' \
  '.[0].jobs[0].status="in_progress"' \
  '.[0].jobs += .[0].jobs'; do
  if JOB_MUTATION="$mutation" run_check; then echo "accepted invalid gate: $mutation" >&2; exit 1; fi
done
if REQUESTED_REF=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa run_check; then cat "$fixture/output" >&2; echo 'accepted wrong source' >&2; exit 1; fi
if FETCH_FAIL=true run_check; then echo 'accepted failed fetch' >&2; exit 1; fi
if API_FAIL=true run_check; then echo 'accepted unavailable API' >&2; exit 1; fi
# Execute the actual workflow shell line with GitHub's default bash -e mode.
step="$(sed -n 's/^        run: \(.*release-eligibility-check.sh.*\)$/\1/p' "$root/.github/workflows/docker-images.yml")"
[[ -n "$step" ]]
mkdir -p "$fixture/repo/scripts"
cp "$root/scripts/release-eligibility-check.sh" "$fixture/repo/scripts/"
chmod +x "$fixture/repo/scripts/release-eligibility-check.sh"
if (cd "$fixture/repo" && JOB_MUTATION='.[0].jobs=[]' GITHUB_OUTPUT="$fixture/job-output" bash -e -c "$step"); then
  echo 'workflow caller swallowed missing CI Gate' >&2; exit 1
fi
[[ ! -s "$fixture/job-output" ]]
git -C "$fixture/repo" update-ref refs/remotes/origin/main "$commit"
printf 'dirty\n' > "$fixture/repo/tracked"
git -C "$fixture/repo" add tracked
if run_check; then echo 'accepted staged changes' >&2; exit 1; fi
printf 'Release eligibility same-commit provenance tests passed\n'
