#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
ci_workflow="${repo_root}/.github/workflows/ci.yml"
docker_workflow="${repo_root}/.github/workflows/docker-images.yml"
release_workflow="${repo_root}/.github/workflows/release.yml"
backend_workflow="${repo_root}/.github/workflows/backend.yml"
ops_workflow="${repo_root}/.github/workflows/ops.yml"

assert_contains() {
  local file="$1"
  local expected="$2"

  if ! grep -Fq -- "${expected}" "${file}"; then
    printf 'Expected %s to contain %q\n' "${file}" "${expected}" >&2
    exit 1
  fi
}

assert_not_contains() {
  local file="$1"
  local unexpected="$2"

  if grep -Fq -- "${unexpected}" "${file}"; then
    printf 'Expected %s not to contain %q\n' "${file}" "${unexpected}" >&2
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
assert_contains "${ops_workflow}" '  workflow_call:'
assert_contains "${ops_workflow}" 'uses: ./.github/actions/setup-go'
assert_contains "${ops_workflow}" 'run: go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/*.yml'
assert_contains "${ops_workflow}" 'run: bash scripts/deploy-stack-test.sh'
assert_contains "${backend_workflow}" '  workflow_call:'
assert_contains "${backend_workflow}" 'name: Backend Verification'
assert_contains "${backend_workflow}" '          ref: ${{ inputs.ref || github.sha }}'
assert_contains "${backend_workflow}" '      - name: Resolve Schema Release commits'
assert_contains "${backend_workflow}" '          REQUESTED_BASE_SHA: ${{ inputs.base_sha }}'
assert_contains "${backend_workflow}" '          EVENT_BASE_SHA: ${{ github.event.before }}'
assert_contains "${backend_workflow}" '          if [[ -z "${base_sha}" || "${base_sha}" =~ ^0+$ ]]; then'
assert_contains "${backend_workflow}" '            base_sha="${head_sha}^1"'
assert_contains "${backend_workflow}" '          base_sha="$(git rev-parse --verify "${base_sha}^{commit}")"'
assert_contains "${backend_workflow}" '          BASE_SHA: ${{ steps.schema-release-commits.outputs.base_sha }}'
assert_contains "${backend_workflow}" '          HEAD_SHA: ${{ steps.schema-release-commits.outputs.head_sha }}'
assert_not_contains "${backend_workflow}" 'inputs.base_sha || github.event.before || github.sha'
assert_contains "${backend_workflow}" '      - name: Check Schema Release contract'
assert_contains "${backend_workflow}" '        run: go run ./cmd/schema-release'
assert_not_contains "${backend_workflow}" 'schema-assemble'
assert_not_contains "${backend_workflow}" 'check-migration-history.sh'
if [[ -e "${repo_root}/apps/api/cmd/schema-assemble" || -e "${repo_root}/scripts/check-migration-history.sh" ]]; then
  echo 'Legacy schema verification entrypoints must be removed' >&2
  exit 1
fi
assert_contains "${repo_root}/.github/workflows/security.yml" '  workflow_call:'
assert_contains "${ci_workflow}" '    name: CI Gate'
assert_contains "${ci_workflow}" '      - security'
assert_contains "${ci_workflow}" '      - ops'
assert_contains "${ci_workflow}" "needs.changes.outputs.api == 'true'"
assert_contains "${ci_workflow}" "needs.changes.outputs.auth == 'true'"
assert_contains "${ci_workflow}" "needs.changes.outputs.docker == 'true'"
assert_contains "${ci_workflow}" "needs.changes.outputs.web != 'true'"
assert_contains "${ci_workflow}" '"${WEB_REQUIRED}" != "true" && "${BROWSER_SMOKE_RESULT}" != "success"'

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
assert_contains "${docker_workflow}" '      base_sha: ${{ github.event.before }}'
assert_contains "${docker_workflow}" '          provenance: mode=max'
assert_contains "${docker_workflow}" '          sbom: true'
assert_contains "${docker_workflow}" '          tags: ${{ steps.image.outputs.immutable_tag }}'
assert_contains "${docker_workflow}" '      - name: Publish verified image aliases'
assert_contains "${docker_workflow}" '  deploy-production:'
assert_contains "${docker_workflow}" "    if: needs.aggregate-release-manifest.outputs.is-production-release == 'true'"
assert_contains "${docker_workflow}" '      - aggregate-release-manifest'
assert_contains "${docker_workflow}" '    environment: production'
assert_contains "${docker_workflow}" '      group: production-deployment'
assert_contains "${docker_workflow}" '      cancel-in-progress: false'
assert_contains "${docker_workflow}" "  cancel-in-progress: \${{ github.ref_type != 'tag' }}"
assert_contains "${docker_workflow}" '          name: release-manifest'
assert_contains "${docker_workflow}" '[[ "${GITHUB_REF_NAME}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]'
assert_contains "${docker_workflow}" '[[ "${GITHUB_EVENT_NAME}" == "push" ]]'
assert_contains "${docker_workflow}" '--arg expected_tag "${GITHUB_REF_NAME}"'
assert_contains "${docker_workflow}" '--arg expected_commit "${GITHUB_SHA}"'
assert_contains "${docker_workflow}" '--arg repository "${GITHUB_REPOSITORY}"'
assert_contains "${docker_workflow}" '--arg expected_repository "${GITHUB_REPOSITORY}"'
assert_contains "${docker_workflow}" '--arg expected_image_repository "ghcr.io/${GITHUB_REPOSITORY,,}"'
assert_contains "${docker_workflow}" 'StrictHostKeyChecking=yes'
assert_contains "${docker_workflow}" 'UserKnownHostsFile="${SSH_KNOWN_HOSTS_FILE}"'
assert_contains "${docker_workflow}" 'PasswordAuthentication=no'
assert_contains "${docker_workflow}" 'ConnectTimeout=15'
assert_contains "${docker_workflow}" '< release-assets/release-manifest.json'
assert_contains "${docker_workflow}" 'production-deploy-result.json'
assert_contains "${docker_workflow}" 'production-deploy-error.raw'
assert_contains "${docker_workflow}" 'recovery_hint'
assert_contains "${docker_workflow}" "jq -c '{result, stage, recovery_hint}'"
assert_contains "${docker_workflow}" '| Failed stage |'
assert_contains "${docker_workflow}" '| Recovery hint |'
assert_contains "${docker_workflow}" 'RESULT: ${{ job.status }}'

deploy_job="$(sed -n '/^  deploy-production:/,$p' "${docker_workflow}")"
if grep -Fq 'curl ' <<< "${deploy_job}" || grep -Fq 'docker build' <<< "${deploy_job}"; then
  echo 'Production deployment must consume the same-run manifest without fetching or rebuilding images' >&2
  exit 1
fi

manifest_validation_line="$(grep -n -F -- '--arg expected_tag "${GITHUB_REF_NAME}"' "${docker_workflow}" | cut -d: -f1)"
ssh_deploy_line="$(grep -n -F -- '< release-assets/release-manifest.json' "${docker_workflow}" | cut -d: -f1)"
if [[ -z "${manifest_validation_line}" || -z "${ssh_deploy_line}" || "${manifest_validation_line}" -ge "${ssh_deploy_line}" ]]; then
  echo 'Production manifest validation must complete before SSH transport' >&2
  exit 1
fi
assert_contains "${docker_workflow}" '          SYFT_PLATFORM: linux/amd64'
assert_contains "${docker_workflow}" '          SYFT_PLATFORM: linux/arm64'
assert_contains "${docker_workflow}" '        uses: ./.github/actions/setup-grype'
assert_contains "${docker_workflow}" 'run: grype "registry:${IMAGE}" --platform linux/amd64'
assert_contains "${docker_workflow}" 'run: grype "registry:${IMAGE}" --platform linux/arm64'
if grep -Fq 'SYFT_PLATFORM: all' "${docker_workflow}"; then
  echo 'Syft requires an explicit operating system and architecture' >&2
  exit 1
fi
if [[ ! -f "${repo_root}/.grype-version" ]]; then
  echo 'Expected .grype-version to be the Grype version source of truth' >&2
  exit 1
fi
if [[ ! -f "${repo_root}/.grype-checksums" ]]; then
  echo 'Expected .grype-checksums to pin the trusted Grype archive' >&2
  exit 1
fi
assert_contains "${repo_root}/.github/actions/setup-grype/action.yml" 'sha256sum --check'
assert_contains "${repo_root}/.github/actions/setup-grype/action.yml" 'releases/download/v${version}/${archive_name}'
if grep -Fq 'anchore/scan-action@' "${docker_workflow}"; then
  echo 'Docker scanning must use the verified repository Grype binary' >&2
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
assert_contains "${release_workflow}" '              --paginate \'
assert_contains "${release_workflow}" '              --slurp \'
assert_contains "${release_workflow}" 'check-runs?per_page=100'
assert_contains "${release_workflow}" 'jq '\''[.[].check_runs[] | select(.name == "CI Gate" and .conclusion == "success")] | length'\'''
assert_contains "${release_workflow}" '          if [[ -f CHANGELOG.md ]]; then'

if grep -Eq '^  backend-verification:' "${release_workflow}"; then
  echo 'Release workflow must reuse the successful CI Gate instead of repeating Backend Verification' >&2
  exit 1
fi

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
