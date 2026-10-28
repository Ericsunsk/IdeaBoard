#!/usr/bin/env bash
# Only successful ancestor CI runs can certify the start of an incremental range.
set -euo pipefail
changed_paths="${1:?usage: ci-baseline.sh changed-paths-file}"
head_sha="$(git rev-parse 'HEAD^{commit}')"
base_sha=''
force_full=true
requested_full="${FORCE_FULL:-false}"

full_verification() {
  printf '%s\n' .github/workflows/ci.yml > "${changed_paths}"
  printf 'ci-baseline: full verification (%s)\n' "$1" >&2
}

find_successful_ancestor() {
  local response candidate count page branch encoded_branch
  branch="${BASE_BRANCH:-}"
  [[ -n "${branch}" && "${GITHUB_REPOSITORY:-}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || return 1
  encoded_branch="$(jq -rn --arg branch "${branch}" '$branch | @uri')"
  for page in 1 2 3 4 5; do
    if ! response="$(gh api "/repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?branch=${encoded_branch}&event=push&status=success&per_page=100&page=${page}" 2>/dev/null)"; then
      return 1
    fi
    jq -e '.workflow_runs | type == "array"' >/dev/null <<<"${response}" || return 1
    while IFS= read -r candidate; do
      [[ "${candidate}" =~ ^[0-9a-f]{40}$ ]] || continue
      if git cat-file -e "${candidate}^{commit}" 2>/dev/null && git merge-base --is-ancestor "${candidate}" "${head_sha}"; then
        base_sha="${candidate}"
        return 0
      fi
    done < <(jq -r --arg repository "${GITHUB_REPOSITORY}" --arg branch "${branch}" '.workflow_runs[] | select(.conclusion == "success" and .event == "push" and .head_branch == $branch and .head_repository.full_name == $repository) | .head_sha' <<<"${response}")
    count="$(jq '.workflow_runs | length' <<<"${response}")"
    [[ "${count}" -eq 100 ]] || break
  done
  return 1
}

if [[ -n "${SOURCE_COMMIT:-}" && ( ! "${SOURCE_COMMIT}" =~ ^[0-9a-f]{40}$ || "${SOURCE_COMMIT}" != "${head_sha}" ) ]]; then
  echo 'ci-baseline: requested source commit does not match checkout' >&2
  exit 1
fi
if [[ "${requested_full}" != true && "${requested_full}" != false ]]; then
  echo 'ci-baseline: FORCE_FULL must be true or false' >&2
  exit 1
fi
if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
  full_verification 'checkout is dirty'
elif [[ "$(git rev-parse --is-shallow-repository)" == true ]]; then
  full_verification 'history is incomplete'
elif find_successful_ancestor; then
  if [[ "${requested_full}" == true ]]; then
    # Full test scope still needs the certified comparison base for migration
    # risk/OpenAPI review. The successful source itself is a valid base.
    full_verification 'explicit full mode with a verified comparison base'
  elif git diff --no-renames --name-only "${base_sha}" "${head_sha}" > "${changed_paths}"; then
    force_full=false
    printf 'ci-baseline: incremental range %s..%s\n' "${base_sha}" "${head_sha}" >&2
  else
    base_sha=''
    full_verification 'cannot inspect the verified range'
  fi
else
  full_verification 'no available successful ancestor; history-sensitive gates must fail closed'
fi
printf 'base_sha=%s\nhead_sha=%s\nforce_full=%s\n' "${base_sha}" "${head_sha}" "${force_full}"
