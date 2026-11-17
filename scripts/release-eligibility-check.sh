#!/usr/bin/env bash
# A release consumes successful canonical main CI for this exact source commit.
set -euo pipefail
: "${REQUESTED_REF:?REQUESTED_REF is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
[[ "${GITHUB_REPOSITORY}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || exit 1
source_commit="$(git rev-parse 'HEAD^{commit}')" || exit 1
if [[ "${REQUESTED_REF}" =~ ^v ]]; then
  [[ "${REQUESTED_REF}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 1
  [[ "$(git rev-parse "refs/tags/${REQUESTED_REF}^{commit}")" == "${source_commit}" ]] || exit 1
else
  [[ "${REQUESTED_REF}" =~ ^[0-9a-f]{40}$ && "${REQUESTED_REF}" == "${source_commit}" ]] || exit 1
fi
git diff --quiet || exit 1
git diff --cached --quiet || exit 1
git -c credential.helper= -c 'credential.helper=!gh auth git-credential' fetch origin main --no-tags || exit 1
git merge-base --is-ancestor "${source_commit}" origin/main || exit 1

# Check run status below; GitHub's status index can omit completed runs.
runs="$(gh api --paginate --slurp "/repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?head_sha=${source_commit}&branch=main&event=push&per_page=100")" || exit 1
candidate_ids="$(jq -er --arg commit "${source_commit}" --arg repository "${GITHUB_REPOSITORY}" '
  [.[].workflow_runs[] | select(
    .head_sha == $commit and .head_branch == "main" and .event == "push" and
    .head_repository.full_name == $repository and .path == ".github/workflows/ci.yml" and
    .status == "completed" and .conclusion == "success" and
    (.id | type == "number" and . > 0 and floor == .)
  ) | .id] | if length == 0 then error("No successful canonical main CI for this commit") else .[] end
' <<<"${runs}")" || exit 1
while IFS= read -r run_id; do
  jobs="$(gh api --paginate --slurp "/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}/jobs?filter=latest&per_page=100")" || exit 1
  if jq -e --arg commit "${source_commit}" '
    [.[].jobs[] | select(.name == "CI Gate" and .head_sha == $commit and
      .status == "completed" and .conclusion == "success")] | length == 1
  ' >/dev/null <<<"${jobs}"; then
    printf 'source_commit=%s\ntrusted_ci_run_id=%s\n' "${source_commit}" "${run_id}"
    exit 0
  fi
done <<<"${candidate_ids}"
printf 'Release requires a successful CI Gate in the same-commit canonical main push run\n' >&2
exit 1
