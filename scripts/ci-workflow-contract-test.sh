#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ci="${root}/.github/workflows/ci.yml"
docker="${root}/.github/workflows/docker-images.yml"
assert_contains() { grep -Fq -- "$2" "$1" || { echo "Expected $1 to contain $2" >&2; exit 1; }; }
assert_not_contains() { ! grep -Fq -- "$2" "$1" || { echo "Expected $1 not to contain $2" >&2; exit 1; }; }

assert_contains "${ci}" '  pull_request:'
assert_contains "${ci}" '    branches: [main]'
assert_not_contains "${ci}" 'merge_group:'
assert_contains "${ci}" 'name: CI Gate'
assert_contains "${ci}" 'uses: ./.github/workflows/api.yml'
assert_contains "${ci}" 'uses: ./.github/workflows/auth.yml'
assert_contains "${ci}" 'uses: ./.github/workflows/migration-integration.yml'
assert_not_contains "${ci}" 'security:'

for workflow in web.yml api.yml auth.yml migration-integration.yml ops.yml; do
  assert_contains "${root}/.github/workflows/${workflow}" '  workflow_call:'
done
assert_contains "${root}/.github/workflows/api.yml" 'TEST_MODE: race'
assert_not_contains "${root}/.github/workflows/api.yml" 'mode: [standard, race]'
assert_contains "${root}/.github/workflows/migration-integration.yml" 'npm run migrate:dist'
assert_contains "${root}/.github/workflows/web.yml" 'npm audit --omit=dev --audit-level=high'

assert_contains "${docker}" '      - preflight'
assert_contains "${docker}" 'checks: read'
assert_contains "${docker}" 'CI Gate'
assert_contains "${docker}" 'immutable_tag=ghcr.io/${repository}/ideaboard-${{ matrix.image.name }}:sha-${{ needs.preflight.outputs.source_commit }}'
assert_not_contains "${docker}" 'latest'
assert_not_contains "${docker}" 'image_tag'
assert_not_contains "${docker}" 'Publish verified image aliases'
assert_not_contains "${docker}" 'uses: ./.github/workflows/backend.yml'
assert_contains "${docker}" 'provenance: mode=max'
assert_contains "${docker}" 'sbom: true'
assert_contains "${docker}" 'StrictHostKeyChecking=yes'
assert_contains "${docker}" 'UserKnownHostsFile="${SSH_KNOWN_HOSTS_FILE}"'
assert_contains "${docker}" 'PasswordAuthentication=no'
assert_contains "${docker}" '< release-assets/release-manifest.json'
assert_contains "${docker}" 'production-deploy-error.raw'

if [[ -e "${root}/.github/workflows/backend.yml" || -e "${root}/.github/workflows/release.yml" ]]; then
  echo 'Legacy backend/release workflows must be removed' >&2
  exit 1
fi
if grep -R -Eq '^  (pull_request|push|merge_group):' "${root}/.github/workflows/web.yml" "${root}/.github/workflows/api.yml" "${root}/.github/workflows/auth.yml"; then
  echo 'Reusable component workflows must not expose event triggers' >&2
  exit 1
fi
if grep -R -Eq 'uses: actions/setup-(node|go)@|node-version:' "${root}/.github/workflows"; then
  echo 'Workflows must use repository setup actions and .node-version' >&2
  exit 1
fi
while IFS= read -r file; do
  while IFS= read -r line; do
    ref="${line#*uses: }"
    [[ "${ref}" == ./* ]] && continue
    [[ "${ref}" =~ ^[^@]+@[0-9a-f]{40}$ ]] || { echo "External action must use full SHA: ${ref}" >&2; exit 1; }
  done < <(grep -E '^[[:space:]]+uses: ' "${file}")
done < <(find "${root}/.github/workflows" "${root}/.github/actions" -type f -name '*.yml' -print)

echo 'CI workflow contract tests passed'
