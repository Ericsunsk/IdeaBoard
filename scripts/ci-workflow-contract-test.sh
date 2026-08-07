#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
ci_workflow="${repo_root}/.github/workflows/ci.yml"
docker_workflow="${repo_root}/.github/workflows/docker-images.yml"
release_workflow="${repo_root}/.github/workflows/release.yml"

assert_contains() {
  local file="$1"
  local expected="$2"

  if ! grep -Fq -- "${expected}" "${file}"; then
    printf 'Expected %s to contain %q\n' "${file}" "${expected}" >&2
    exit 1
  fi
}

if [[ ! -f "${ci_workflow}" ]]; then
  echo 'Expected an always-on CI workflow at .github/workflows/ci.yml' >&2
  exit 1
fi

assert_contains "${ci_workflow}" 'name: CI Gate'
assert_contains "${ci_workflow}" '  pull_request:'
assert_contains "${ci_workflow}" '  merge_group:'

if grep -Fq '    paths:' "${ci_workflow}"; then
  echo 'The always-on CI workflow must not use path filters' >&2
  exit 1
fi

assert_contains "${repo_root}/.github/workflows/web.yml" '  workflow_call:'
assert_contains "${repo_root}/.github/workflows/ops.yml" '  workflow_call:'
assert_contains "${repo_root}/.github/workflows/backend.yml" '  workflow_call:'
assert_contains "${repo_root}/.github/workflows/backend.yml" 'name: Backend Verification'
assert_contains "${repo_root}/.github/workflows/backend.yml" '          ref: ${{ inputs.ref || github.sha }}'
assert_contains "${repo_root}/.github/workflows/security.yml" '  workflow_call:'
assert_contains "${ci_workflow}" '    name: CI Gate'
assert_contains "${ci_workflow}" '      - security'
assert_contains "${ci_workflow}" '      - ops'
assert_contains "${ci_workflow}" "needs.changes.outputs.api == 'true'"
assert_contains "${ci_workflow}" "needs.changes.outputs.auth == 'true'"
assert_contains "${ci_workflow}" "needs.changes.outputs.docker == 'true'"

if grep -R -Fq 'secrets: inherit' "${repo_root}/.github/workflows"; then
  echo 'Reusable workflows must not inherit unrelated repository secrets' >&2
  exit 1
fi

for reusable_workflow in web.yml ops.yml backend.yml; do
  if grep -Eq '^  (pull_request|push|merge_group):' \
    "${repo_root}/.github/workflows/${reusable_workflow}"; then
    echo "Reusable workflow ${reusable_workflow} must not expose a legacy event trigger" >&2
    exit 1
  fi
done

if grep -Eq '^[[:space:]]+pull_request:' "${docker_workflow}"; then
  echo 'Docker publishing must not run as a legacy pull request workflow' >&2
  exit 1
fi

for legacy_job in api-generated api-migrations api-tests api-quality auth verify build-pr-images; do
  if grep -Eq "^  ${legacy_job}:" "${docker_workflow}"; then
    echo "Docker publishing still contains legacy verification job ${legacy_job}" >&2
    exit 1
  fi
done

assert_contains "${docker_workflow}" '    uses: ./.github/workflows/backend.yml'
assert_contains "${docker_workflow}" '          provenance: mode=max'
assert_contains "${docker_workflow}" '          sbom: true'
assert_contains "${docker_workflow}" '          tags: ${{ steps.image.outputs.immutable_tag }}'
assert_contains "${docker_workflow}" '      - name: Publish verified image aliases'
assert_contains "${docker_workflow}" '          SYFT_PLATFORM: linux/amd64'
assert_contains "${docker_workflow}" '          SYFT_PLATFORM: linux/arm64'
if grep -Fq 'SYFT_PLATFORM: all' "${docker_workflow}"; then
  echo 'Syft requires an explicit operating system and architecture' >&2
  exit 1
fi
for npm_workflow in ci.yml web.yml backend.yml security.yml release.yml; do
  assert_contains "${repo_root}/.github/workflows/${npm_workflow}" 'uses: ./.github/actions/npm-ci'
  if grep -Fq 'run: npm ci' "${repo_root}/.github/workflows/${npm_workflow}"; then
    echo "Workflow ${npm_workflow} must use the repository npm-ci action" >&2
    exit 1
  fi
done
assert_contains "${release_workflow}" 'permissions:'
assert_contains "${release_workflow}" '  contents: read'
assert_contains "${release_workflow}" '      contents: write'
assert_contains "${release_workflow}" '      ref: ${{ github.event_name == '\''workflow_dispatch'\'' && inputs.tag || github.ref }}'

if sed -n '/^permissions:/,/^[^[:space:]]/p' "${release_workflow}" | grep -Fq 'contents: write'; then
  echo 'Release workflow must grant contents: write only to the publishing job' >&2
  exit 1
fi

if [[ ! -f "${repo_root}/.node-version" ]]; then
  echo 'Expected .node-version to be the CI Node version source of truth' >&2
  exit 1
fi

if grep -R -Fq 'node-version:' "${repo_root}/.github/workflows"; then
  echo 'Workflows must use .node-version instead of literal Node versions' >&2
  exit 1
fi

if grep -R -Eq 'uses: actions/setup-(node|go)@' "${repo_root}/.github/workflows"; then
  echo 'Workflows must use the repository setup-node and setup-go actions' >&2
  exit 1
fi

while IFS= read -r workflow_file; do
  while IFS= read -r uses_line; do
    action_ref="${uses_line#*uses: }"
    action_ref="${action_ref%\"}"
    action_ref="${action_ref#\"}"
    action_ref="${action_ref%\'}"
    action_ref="${action_ref#\'}"

    [[ "${action_ref}" == ./* ]] && continue
    if [[ ! "${action_ref}" =~ ^[^@]+@[0-9a-f]{40}$ ]]; then
      printf 'External action must use a full commit SHA: %s (%s)\n' \
        "${action_ref}" "${workflow_file}" >&2
      exit 1
    fi
  done < <(grep -E '^[[:space:]]+uses: ' "${workflow_file}")
done < <(find "${repo_root}/.github/workflows" "${repo_root}/.github/actions" -type f -name '*.yml' -print)

echo 'CI workflow contract tests passed'
