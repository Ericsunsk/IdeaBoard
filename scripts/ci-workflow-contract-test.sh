#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "${fixture_dir}"' EXIT
(cd "${root}/apps/api" && go build -o "${fixture_dir}/checker" ../../scripts/ci-workflow-contract-check.go)
(cd "${root}/apps/api" && "${fixture_dir}/checker")

# Exercise actual YAML semantics in an isolated minimal repository, never the worktree.
fixture_root="${fixture_dir}/repo"
mkdir -p "${fixture_root}/scripts" "${fixture_root}/apps/api/cmd/migrationhistory"
cp -R "${root}/.github" "${fixture_root}/.github"
for name in atlas-migration-check.sh ci-migration-integration.sh ci-change-classifier.sh run-all-self-tests.sh run-production-deploy.sh; do
  cp "${root}/scripts/${name}" "${fixture_root}/scripts/${name}"
done
cp "${root}/apps/api/cmd/migrationhistory/main.go" "${fixture_root}/apps/api/cmd/migrationhistory/main.go"
(cd "${fixture_root}/apps/api" && "${fixture_dir}/checker")
reject_mutation() {
  local file="$1" expression="$2" expected="$3"
  sed "${expression}" "${root}/${file}" > "${fixture_root}/${file}"
  if (cd "${fixture_root}/apps/api" && "${fixture_dir}/checker") > "${fixture_dir}/failure.log" 2>&1; then
    echo "CI workflow contract accepted regression: ${expected}" >&2
    exit 1
  fi
  grep -Fq "${expected}" "${fixture_dir}/failure.log"
  cp "${root}/${file}" "${fixture_root}/${file}"
}
reject_mutation .github/workflows/docker-images.yml 's/force_full: true/force_full: false/' 'An incremental CI pass cannot authorize publishing'
reject_mutation .github/workflows/docker-images.yml '/      - verify-source/d' 'Publishing must wait for full source verification'
reject_mutation .github/workflows/auth.yml 's/inputs.source_commit || github.sha/github.sha/' 'auth.yml must checkout the exact verified source'
reject_mutation .github/workflows/auth.yml '\#      - uses: ./.github/actions/setup-go#,+2d' 'Auth must use the canonical Go setup'
reject_mutation .github/workflows/auth.yml '/      - name: Prepare Go contract test/,+2d' 'Missing step: Prepare Go contract test'
reject_mutation .github/workflows/auth.yml "s/go test -run '\^\$' .\/internal\/identity\/auth/go version/" 'Auth must compile the Go contract before its runtime deadline'
reject_mutation .github/workflows/auth.yml 's/name: Test Auth$/name: Auth test placeholder/; s/name: Lint Auth$/name: Test Auth/; s/name: Auth test placeholder/name: Lint Auth/' 'Auth must set up and compile Go before running contract assertions'
reject_mutation .github/workflows/auth.yml '/      - name: Prepare Go contract test/a\
        continue-on-error: true
' 'Auth contract preparation failures must fail verification'
reject_mutation .github/workflows/ci.yml 's/run_sqlc: .*/run_sqlc: false/' 'API must consume its precise check requirement'
reject_mutation .github/workflows/ci.yml '/          BROWSER_SMOKE_REQUIRED:/d' 'Browser smoke must use the explicit runtime-impact flag'
reject_mutation .github/workflows/ci.yml 's#./scripts/ci-baseline.sh#./scripts/untrusted-baseline.sh#' 'Incremental CI must use the trusted ancestor resolver'
reject_mutation .github/workflows/api.yml 's/API_TEST_PLAN: .*/API_TEST_PLAN: wrong-plan.json/' 'API shards must consume the single verified plan'
reject_mutation .github/workflows/web.yml 's/CI_FORCE_FULL: .*/CI_FORCE_FULL: false/' 'Web planner must preserve full-release mode'
reject_mutation .github/workflows/api.yml "s/ATLAS_BASE_SHA: .*/ATLAS_BASE_SHA: \${{ github.event.before }}/" 'Migration review must use the cumulative verified baseline'
reject_mutation .github/workflows/api.yml "/- name: Check for breaking OpenAPI changes/,/^  static-contracts:/s/BASE_SHA: .*/BASE_SHA: \${{ github.event.before }}/" 'OpenAPI review must use the cumulative verified baseline'
reject_mutation .github/workflows/api.yml '/A verified ancestor base SHA is required for OpenAPI review/d' 'OpenAPI review must reject missing or invalid comparison history'
echo 'CI workflow negative fixtures passed'
