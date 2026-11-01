#!/usr/bin/env bash
# Only successful canonical tagged releases can supply image reuse candidates.
set -euo pipefail
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${SOURCE_COMMIT:?SOURCE_COMMIT is required}"
: "${RELEASE_TAG:?RELEASE_TAG is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
output="${1:?usage: prepare-release-plan.sh output.json}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
previous_manifest=""
trusted_run() {
  local run_id="$1" commit="$2" tag="$3" data
  [[ "$run_id" =~ ^[1-9][0-9]*$ && "$commit" =~ ^[0-9a-f]{40}$ && "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || return 1
  data="$(gh api "/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}" 2>/dev/null)" || return 1
  jq -e --arg repository "$GITHUB_REPOSITORY" --arg commit "$commit" --arg tag "$tag" '
    .head_repository.full_name == $repository and .head_sha == $commit and .head_branch == $tag and
    .path == ".github/workflows/docker-images.yml" and .event == "push" and
    .status == "completed" and .conclusion == "success"
  ' >/dev/null <<<"$data"
}
fetch_candidate() {
  local release tag commit run_id source_commit source_tag source_run
  release="$(gh api "/repos/${GITHUB_REPOSITORY}/releases/latest" 2>/dev/null)" || return 1
  tag="$(jq -er '.tag_name | strings' <<<"$release")" || return 1
  [[ "$tag" != "$RELEASE_TAG" && "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || return 1
  gh release download "$tag" --repo "$GITHUB_REPOSITORY" --pattern release-manifest.json --dir "$fixture" >/dev/null 2>&1 || return 1
  local previous="$fixture/release-manifest.json"
  jq -e --arg tag "$tag" '.schema_version == 2 and .tag == $tag' "$previous" >/dev/null || return 1
  python3 "$script_dir/release-artifacts.py" validate --manifest "$previous" --repository "$GITHUB_REPOSITORY" >/dev/null 2>&1 || return 1
  commit="$(jq -er '.commit' "$previous")" || return 1
  run_id="$(jq -er '.workflow.run_id | tostring' "$previous")" || return 1
  trusted_run "$run_id" "$commit" "$tag" || return 1
  while IFS=$'\t' read -r source_commit source_tag source_run; do
    trusted_run "$source_run" "$source_commit" "$source_tag" || return 1
  done < <(jq -r '.components[] | [.source_commit,.source_tag,.source_run_id] | @tsv' "$previous" | sort -u)
  previous_manifest="$previous"
}
if ! fetch_candidate; then
  printf 'No compatible trusted release evidence; building complete image set\n' >&2
fi
plan_args=(--repository "$GITHUB_REPOSITORY" --commit "$SOURCE_COMMIT" --tag "$RELEASE_TAG"
  --ref "${RELEASE_REF:-refs/tags/$RELEASE_TAG}" --run-id "$GITHUB_RUN_ID" --output "$output")
if [[ -n "$previous_manifest" ]]; then
  plan_args+=(--previous "$previous_manifest")
fi
python3 "$script_dir/release-artifacts.py" plan "${plan_args[@]}"
