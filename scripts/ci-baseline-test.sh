#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "${fixture_dir}"' EXIT
mkdir -p "${fixture_dir}/bin" "${fixture_dir}/repo"
cat > "${fixture_dir}/bin/gh" <<'MOCK'
#!/usr/bin/env bash
[[ "${FAIL_API:-false}" != true ]] || exit 1
if [[ "${PAGINATE_API:-false}" == true && "$2" == *'&page=2' ]]; then
  cat "${FIXTURE_RESPONSE}.page2"
else
  cat "${FIXTURE_RESPONSE}"
fi
MOCK
chmod +x "${fixture_dir}/bin/gh"
export PATH="${fixture_dir}/bin:${PATH}"
export GIT_AUTHOR_NAME=Fixture GIT_AUTHOR_EMAIL=fixture@example.invalid GIT_COMMITTER_NAME=Fixture GIT_COMMITTER_EMAIL=fixture@example.invalid
export GITHUB_REPOSITORY=owner/repo BASE_BRANCH=main
export FIXTURE_RESPONSE="${fixture_dir}/response.json"
git -C "${fixture_dir}/repo" init -q
cd "${fixture_dir}/repo"
printf 'first\n' > code.go
git add code.go
git commit -qm 'verified ancestor'
verified_sha="$(git rev-parse HEAD)"
printf 'failed change\n' > code.go
git commit -qam 'failed CI commit'
printf 'docs\n' > README.md
git add README.md
git commit -qm 'later unrelated change'
head_sha="$(git rev-parse HEAD)"
response() {
  jq -n --arg commit "$1" --arg repository "${2:-owner/repo}" --arg branch "${3:-main}" '{workflow_runs:[{head_sha:$commit, conclusion:"success", event:"push", head_branch:$branch, head_repository:{full_name:$repository}}]}' > "${FIXTURE_RESPONSE}"
}
run_baseline() {
  "${script_dir}/ci-baseline.sh" "${fixture_dir}/paths" > "${fixture_dir}/outputs"
}
assert_line() { grep -Fqx "$1" "${fixture_dir}/outputs"; }
assert_full() {
  run_baseline
  assert_line force_full=true
  assert_line base_sha=
  grep -Fqx '.github/workflows/ci.yml' "${fixture_dir}/paths"
}
response "${verified_sha}"
run_baseline
assert_line force_full=false
assert_line "base_sha=${verified_sha}"
assert_line "head_sha=${head_sha}"
grep -Fqx code.go "${fixture_dir}/paths"
grep -Fqx README.md "${fixture_dir}/paths"
# The failed preceding commit must remain in the cumulative range.
if grep -Fqx "base_sha=$(git rev-parse HEAD^)" "${fixture_dir}/outputs"; then exit 1; fi
# The resolver must paginate past newer successful runs from unrelated histories.
cp "${FIXTURE_RESPONSE}" "${FIXTURE_RESPONSE}.page2"
jq -n '{workflow_runs:[range(100) | {head_sha:"0000000000000000000000000000000000000000", conclusion:"success", event:"push", head_branch:"main", head_repository:{full_name:"owner/repo"}}]}' > "${FIXTURE_RESPONSE}"
PAGINATE_API=true run_baseline
assert_line "base_sha=${verified_sha}"
response "${verified_sha}"
FAIL_API=true assert_full
response 'invalid-sha'
assert_full
response "${verified_sha}" other/repository
assert_full
response "${verified_sha}" owner/repo wrong-branch
assert_full
unrelated_sha="$(printf 'unrelated\n' | git commit-tree 'HEAD^{tree}')"
response "${unrelated_sha}"
assert_full
printf '{"workflow_runs":{}}\n' > "${FIXTURE_RESPONSE}"
assert_full
response "${verified_sha}"
FORCE_FULL=true run_baseline
assert_line force_full=true
assert_line "base_sha=${verified_sha}"
grep -Fqx '.github/workflows/ci.yml' "${fixture_dir}/paths"
FAIL_API=true FORCE_FULL=true assert_full
# An already-successful release source can serve as its certified review base.
response "${head_sha}"
FORCE_FULL=true run_baseline
assert_line force_full=true
assert_line "base_sha=${head_sha}"
response "${verified_sha}"
SOURCE_COMMIT="${verified_sha}"
if SOURCE_COMMIT="${SOURCE_COMMIT}" run_baseline; then
  echo 'mismatched checkout source was accepted' >&2; exit 1
fi
unset SOURCE_COMMIT
printf 'dirty\n' >> code.go
assert_full
git checkout -- code.go
printf 'untracked\n' > untracked.go
assert_full
rm untracked.go
# A PR merge checkout must use the actual merge commit and base-branch ancestor.
pr_sha="$(printf 'PR branch\n' | git commit-tree 'HEAD^{tree}' -p "${verified_sha}")"
merge_sha="$(printf 'PR merge\n' | git commit-tree 'HEAD^{tree}' -p "${head_sha}" -p "${pr_sha}")"
git update-ref HEAD "${merge_sha}"
response "${head_sha}"
run_baseline
assert_line "base_sha=${head_sha}"
assert_line "head_sha=${merge_sha}"
# Deletions and both rename endpoints remain visible, including on a new branch.
git checkout -qb new-worker-branch
mkdir -p apps/ai-proxy/src
git mv code.go apps/ai-proxy/src/renamed.ts
git rm -q README.md
git commit -qm 'rename Worker input and delete documentation'
response "${head_sha}"
run_baseline
assert_line force_full=false
assert_line "base_sha=${head_sha}"
grep -Fqx code.go "${fixture_dir}/paths"
grep -Fqx apps/ai-proxy/src/renamed.ts "${fixture_dir}/paths"
grep -Fqx README.md "${fixture_dir}/paths"
# Shallow history cannot establish coverage safely.
git clone -q --depth=1 "file://${fixture_dir}/repo" "${fixture_dir}/shallow"
cd "${fixture_dir}/shallow"
assert_full
printf 'CI trusted baseline contract tests passed\n'
