#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
classifier="${script_dir}/ci-change-classifier.sh"

assert_output_line() {
  local output="$1"
  local expected="$2"

  if ! grep -Fqx -- "${expected}" <<<"${output}"; then
    printf 'Expected classifier output to contain %q, got:\n%s\n' "${expected}" "${output}" >&2
    exit 1
  fi
}

docs_output="$(printf '%s\n' README.md docs/runbooks/deployment.md | "${classifier}")"

assert_output_line "${docs_output}" 'web=false'
assert_output_line "${docs_output}" 'api=false'
assert_output_line "${docs_output}" 'auth=false'
assert_output_line "${docs_output}" 'realtime=false'
assert_output_line "${docs_output}" 'docker=false'
assert_output_line "${docs_output}" 'ops=false'
assert_output_line "${docs_output}" 'migration_integration=false'
assert_output_line "${docs_output}" 'docs_only=true'

web_output="$(printf '%s\n' apps/web/src/main.ts | "${classifier}")"

assert_output_line "${web_output}" 'web=true'
assert_output_line "${web_output}" 'api=false'
assert_output_line "${web_output}" 'auth=false'
assert_output_line "${web_output}" 'realtime=false'
assert_output_line "${web_output}" 'docker=false'
assert_output_line "${web_output}" 'docs_only=false'

services_output="$(printf '%s\n' apps/api/internal/tasking/service.go apps/auth/src/index.ts | "${classifier}")"

assert_output_line "${services_output}" 'api=true'
assert_output_line "${services_output}" 'auth=true'
assert_output_line "${services_output}" 'realtime=false'
assert_output_line "${services_output}" 'docker=false'
assert_output_line "${services_output}" 'web=false'
assert_output_line "${services_output}" 'docs_only=false'

realtime_output="$(printf '%s\n' apps/realtime/src/index.ts | "${classifier}")"

assert_output_line "${realtime_output}" 'realtime=true'
assert_output_line "${realtime_output}" 'web=false'
assert_output_line "${realtime_output}" 'api=false'
assert_output_line "${realtime_output}" 'auth=false'
assert_output_line "${realtime_output}" 'docker=false'
assert_output_line "${realtime_output}" 'docs_only=false'

ops_output="$(printf '%s\n' deploy/templates/nginx/ideaboard.conf.template scripts/deploy-stack.sh | "${classifier}")"

assert_output_line "${ops_output}" 'ops=true'
assert_output_line "${ops_output}" 'web=false'
assert_output_line "${ops_output}" 'api=false'
assert_output_line "${ops_output}" 'auth=false'
assert_output_line "${ops_output}" 'realtime=false'
assert_output_line "${ops_output}" 'docker=false'
assert_output_line "${ops_output}" 'docs_only=false'

ci_output="$(printf '%s\n' .github/workflows/ci.yml | "${classifier}")"

assert_output_line "${ci_output}" 'web=true'
assert_output_line "${ci_output}" 'api=true'
assert_output_line "${ci_output}" 'auth=true'
assert_output_line "${ci_output}" 'realtime=true'
assert_output_line "${ci_output}" 'docker=true'
assert_output_line "${ci_output}" 'ops=true'
assert_output_line "${ci_output}" 'migration_integration=true'
assert_output_line "${ci_output}" 'docs_only=false'

docker_workflow_output="$(printf '%s\n' .github/workflows/docker-images.yml | "${classifier}")"

assert_output_line "${docker_workflow_output}" 'web=false'
assert_output_line "${docker_workflow_output}" 'api=false'
assert_output_line "${docker_workflow_output}" 'auth=false'
assert_output_line "${docker_workflow_output}" 'realtime=false'
assert_output_line "${docker_workflow_output}" 'docker=false'
assert_output_line "${docker_workflow_output}" 'ops=true'
assert_output_line "${docker_workflow_output}" 'docs_only=false'

deploy_workflow_output="$(printf '%s\n' .github/workflows/deploy-production.yml | "${classifier}")"

assert_output_line "${deploy_workflow_output}" 'web=false'
assert_output_line "${deploy_workflow_output}" 'api=false'
assert_output_line "${deploy_workflow_output}" 'auth=false'
assert_output_line "${deploy_workflow_output}" 'realtime=false'
assert_output_line "${deploy_workflow_output}" 'docker=false'
assert_output_line "${deploy_workflow_output}" 'ops=true'
assert_output_line "${deploy_workflow_output}" 'docs_only=false'

api_workflow_output="$(printf '%s\n' .github/workflows/api.yml | "${classifier}")"

assert_output_line "${api_workflow_output}" 'web=false'
assert_output_line "${api_workflow_output}" 'api=true'
assert_output_line "${api_workflow_output}" 'auth=false'
assert_output_line "${api_workflow_output}" 'realtime=false'
assert_output_line "${api_workflow_output}" 'docker=true'
assert_output_line "${api_workflow_output}" 'ops=true'

web_workflow_output="$(printf '%s\n' .github/workflows/web.yml | "${classifier}")"

assert_output_line "${web_workflow_output}" 'web=true'
assert_output_line "${web_workflow_output}" 'api=false'
assert_output_line "${web_workflow_output}" 'auth=false'
assert_output_line "${web_workflow_output}" 'realtime=false'
assert_output_line "${web_workflow_output}" 'docker=false'
assert_output_line "${web_workflow_output}" 'ops=true'

auth_workflow_output="$(printf '%s\n' .github/workflows/auth.yml | "${classifier}")"

assert_output_line "${auth_workflow_output}" 'web=false'
assert_output_line "${auth_workflow_output}" 'api=false'
assert_output_line "${auth_workflow_output}" 'auth=true'
assert_output_line "${auth_workflow_output}" 'realtime=false'
assert_output_line "${auth_workflow_output}" 'docker=true'
assert_output_line "${auth_workflow_output}" 'ops=true'

realtime_workflow_output="$(printf '%s\n' .github/workflows/realtime.yml | "${classifier}")"

assert_output_line "${realtime_workflow_output}" 'web=false'
assert_output_line "${realtime_workflow_output}" 'api=false'
assert_output_line "${realtime_workflow_output}" 'auth=false'
assert_output_line "${realtime_workflow_output}" 'realtime=true'
assert_output_line "${realtime_workflow_output}" 'docker=true'
assert_output_line "${realtime_workflow_output}" 'ops=true'

migration_workflow_output="$(printf '%s\n' .github/workflows/migration-integration.yml | "${classifier}")"

assert_output_line "${migration_workflow_output}" 'web=false'
assert_output_line "${migration_workflow_output}" 'api=false'
assert_output_line "${migration_workflow_output}" 'auth=false'
assert_output_line "${migration_workflow_output}" 'realtime=false'
assert_output_line "${migration_workflow_output}" 'docker=false'
assert_output_line "${migration_workflow_output}" 'migration_integration=true'
assert_output_line "${migration_workflow_output}" 'ops=true'

openapi_public_output="$(printf '%s\n' apps/api/openapi.public.yaml | "${classifier}")"

assert_output_line "${openapi_public_output}" 'web=true'
assert_output_line "${openapi_public_output}" 'api=true'
assert_output_line "${openapi_public_output}" 'auth=false'
assert_output_line "${openapi_public_output}" 'realtime=false'
assert_output_line "${openapi_public_output}" 'docs_only=false'

openapi_control_output="$(printf '%s\n' apps/api/openapi.control.yaml | "${classifier}")"

assert_output_line "${openapi_control_output}" 'web=true'
assert_output_line "${openapi_control_output}" 'api=true'
assert_output_line "${openapi_control_output}" 'auth=false'
assert_output_line "${openapi_control_output}" 'realtime=false'
assert_output_line "${openapi_control_output}" 'docs_only=false'

openapi_events_output="$(printf '%s\n' apps/api/openapi.events.yaml | "${classifier}")"

assert_output_line "${openapi_events_output}" 'web=true'
assert_output_line "${openapi_events_output}" 'api=true'
assert_output_line "${openapi_events_output}" 'auth=false'
assert_output_line "${openapi_events_output}" 'realtime=false'
assert_output_line "${openapi_events_output}" 'docs_only=false'

atlas_hcl_output="$(printf '%s\n' apps/api/atlas.hcl | "${classifier}")"

assert_output_line "${atlas_hcl_output}" 'api=true'
assert_output_line "${atlas_hcl_output}" 'migration_integration=true'
assert_output_line "${atlas_hcl_output}" 'web=false'
assert_output_line "${atlas_hcl_output}" 'realtime=false'
assert_output_line "${atlas_hcl_output}" 'docs_only=false'

migration_sql_output="$(printf '%s\n' apps/api/internal/db/migrations/20261108000000_baseline.sql | "${classifier}")"

assert_output_line "${migration_sql_output}" 'api=true'
assert_output_line "${migration_sql_output}" 'migration_integration=true'
assert_output_line "${migration_sql_output}" 'web=false'
assert_output_line "${migration_sql_output}" 'realtime=false'
assert_output_line "${migration_sql_output}" 'docs_only=false'

atlas_sum_output="$(printf '%s\n' apps/api/internal/db/migrations/atlas.sum | "${classifier}")"

assert_output_line "${atlas_sum_output}" 'api=true'
assert_output_line "${atlas_sum_output}" 'migration_integration=true'
assert_output_line "${atlas_sum_output}" 'web=false'
assert_output_line "${atlas_sum_output}" 'realtime=false'
assert_output_line "${atlas_sum_output}" 'docs_only=false'

schema_output="$(printf '%s\n' apps/api/internal/db/schema/schema.sql | "${classifier}")"

assert_output_line "${schema_output}" 'api=true'
assert_output_line "${schema_output}" 'migration_integration=true'
assert_output_line "${schema_output}" 'web=false'
assert_output_line "${schema_output}" 'realtime=false'
assert_output_line "${schema_output}" 'docs_only=false'

for migration_path in \
  apps/api/cmd/migrator/main.go \
  apps/api/cmd/migrationhistory/main.go \
  apps/api/cmd/schemarelease/main.go \
  apps/api/internal/runtime/infrastructure/migration/runner.go \
  apps/api/internal/runtime/composition/wiring/migration.go \
  apps/api/internal/db/migrationhistory/history.go \
  apps/api/internal/db/schemaassembly/assembly.go \
  apps/api/internal/db/schemacontract/contract.go \
  apps/api/internal/db/schemarelease/release.go; do
  migration_tooling_output="$(printf '%s\n' "${migration_path}" | "${classifier}")"
  assert_output_line "${migration_tooling_output}" 'api=true'
  assert_output_line "${migration_tooling_output}" 'migration_integration=true'
  assert_output_line "${migration_tooling_output}" 'web=false'
  assert_output_line "${migration_tooling_output}" 'auth=false'
  assert_output_line "${migration_tooling_output}" 'realtime=false'
  assert_output_line "${migration_tooling_output}" 'docker=false'
  assert_output_line "${migration_tooling_output}" 'docs_only=false'
done

auth_migration_output="$(printf '%s\n' apps/auth/src/infrastructure/authMigration.ts | "${classifier}")"

assert_output_line "${auth_migration_output}" 'auth=true'
assert_output_line "${auth_migration_output}" 'docker=true'
assert_output_line "${auth_migration_output}" 'migration_integration=true'
assert_output_line "${auth_migration_output}" 'web=false'
assert_output_line "${auth_migration_output}" 'api=false'
assert_output_line "${auth_migration_output}" 'realtime=false'
assert_output_line "${auth_migration_output}" 'docs_only=false'

queries_output="$(printf '%s\n' apps/api/internal/db/queries/users.sql | "${classifier}")"

assert_output_line "${queries_output}" 'api=true'
assert_output_line "${queries_output}" 'migration_integration=false'
assert_output_line "${queries_output}" 'web=false'
assert_output_line "${queries_output}" 'realtime=false'
assert_output_line "${queries_output}" 'docs_only=false'

docker_output="$(printf '%s\n' docker/compose.yml docker/.env | "${classifier}")"

assert_output_line "${docker_output}" 'api=false'
assert_output_line "${docker_output}" 'auth=false'
assert_output_line "${docker_output}" 'realtime=false'
assert_output_line "${docker_output}" 'docker=false'
assert_output_line "${docker_output}" 'ops=true'
assert_output_line "${docker_output}" 'migration_integration=true'

dependency_output="$(printf '%s\n' apps/web/package-lock.json apps/api/go.sum | "${classifier}")"

assert_output_line "${dependency_output}" 'web=true'
assert_output_line "${dependency_output}" 'api=true'
assert_output_line "${dependency_output}" 'auth=false'
assert_output_line "${dependency_output}" 'realtime=false'
assert_output_line "${dependency_output}" 'docker=true'

realtime_dependency_output="$(printf '%s\n' apps/realtime/package-lock.json | "${classifier}")"

assert_output_line "${realtime_dependency_output}" 'realtime=true'
assert_output_line "${realtime_dependency_output}" 'realtime_audit=true'
assert_output_line "${realtime_dependency_output}" 'docker=true'
assert_output_line "${realtime_dependency_output}" 'web=false'
assert_output_line "${realtime_dependency_output}" 'api=false'
assert_output_line "${realtime_dependency_output}" 'auth=false'

parity_output="$(printf '%s\n' scripts/ci-parity.sh | "${classifier}")"

assert_output_line "${parity_output}" 'web=true'
assert_output_line "${parity_output}" 'api=true'
assert_output_line "${parity_output}" 'auth=true'
assert_output_line "${parity_output}" 'realtime=true'
assert_output_line "${parity_output}" 'docker=true'
assert_output_line "${parity_output}" 'ops=true'

migration_integration_output="$(printf '%s\n' scripts/ci-migration-integration.sh | "${classifier}")"

assert_output_line "${migration_integration_output}" 'web=false'
assert_output_line "${migration_integration_output}" 'realtime=false'
assert_output_line "${migration_integration_output}" 'api=true'
assert_output_line "${migration_integration_output}" 'auth=true'
assert_output_line "${migration_integration_output}" 'migration_integration=true'
assert_output_line "${migration_integration_output}" 'ops=true'

atlas_migration_output="$(printf '%s\n' scripts/atlas-migration-check.sh | "${classifier}")"

assert_output_line "${atlas_migration_output}" 'api=true'
assert_output_line "${atlas_migration_output}" 'auth=true'
assert_output_line "${atlas_migration_output}" 'realtime=false'
assert_output_line "${atlas_migration_output}" 'migration_integration=true'
assert_output_line "${atlas_migration_output}" 'ops=true'

contract_guard_output="$(printf '%s\n' scripts/sql-workspace-scope-check.sh scripts/openapi-pattern-check.sh scripts/check-backend-copy.sh | "${classifier}")"

assert_output_line "${contract_guard_output}" 'api=true'
assert_output_line "${contract_guard_output}" 'web=false'
assert_output_line "${contract_guard_output}" 'realtime=false'
assert_output_line "${contract_guard_output}" 'ops=true'

gate_output="$(printf '%s\n' scripts/ci-gate.sh | "${classifier}")"

assert_output_line "${gate_output}" 'web=true'
assert_output_line "${gate_output}" 'api=true'
assert_output_line "${gate_output}" 'auth=true'
assert_output_line "${gate_output}" 'realtime=true'
assert_output_line "${gate_output}" 'migration_integration=true'
assert_output_line "${gate_output}" 'docker=true'
assert_output_line "${gate_output}" 'ops=true'

runtime_output="$(printf '%s\n' .node-version | "${classifier}")"

assert_output_line "${runtime_output}" 'web=true'
assert_output_line "${runtime_output}" 'api=false'
assert_output_line "${runtime_output}" 'auth=true'
assert_output_line "${runtime_output}" 'realtime=true'
assert_output_line "${runtime_output}" 'docker=false'
assert_output_line "${runtime_output}" 'ops=true'
assert_output_line "${runtime_output}" 'migration_integration=true'

api_dockerfile_output="$(printf '%s\n' apps/api/Dockerfile | "${classifier}")"

assert_output_line "${api_dockerfile_output}" 'api=true'
assert_output_line "${api_dockerfile_output}" 'auth=false'
assert_output_line "${api_dockerfile_output}" 'realtime=false'
assert_output_line "${api_dockerfile_output}" 'docker=true'

realtime_dockerfile_output="$(printf '%s\n' apps/realtime/Dockerfile | "${classifier}")"

assert_output_line "${realtime_dockerfile_output}" 'realtime=true'
assert_output_line "${realtime_dockerfile_output}" 'api=false'
assert_output_line "${realtime_dockerfile_output}" 'auth=false'
assert_output_line "${realtime_dockerfile_output}" 'docker=true'

unknown_output="$(printf '%s\n' tooling/new-config.toml | "${classifier}")"

assert_output_line "${unknown_output}" 'web=true'
assert_output_line "${unknown_output}" 'api=true'
assert_output_line "${unknown_output}" 'auth=true'
assert_output_line "${unknown_output}" 'realtime=true'
assert_output_line "${unknown_output}" 'docker=true'
assert_output_line "${unknown_output}" 'ops=true'
assert_output_line "${unknown_output}" 'docs_only=false'

env_surface_output="$(printf '%s\n' apps/api/internal/platform/config/config.go apps/api/.env.example apps/auth/src/infrastructure/env.ts apps/auth/.env.example apps/realtime/src/infrastructure/env.ts apps/realtime/.env.example | "${classifier}")"

assert_output_line "${env_surface_output}" 'api=true'
assert_output_line "${env_surface_output}" 'auth=true'
assert_output_line "${env_surface_output}" 'realtime=true'
assert_output_line "${env_surface_output}" 'ops=true'
assert_output_line "${env_surface_output}" 'docs_only=false'

for app_doc in apps/api/internal/identity/CONTEXT.md apps/web/src/features/canvas/README.md; do
  doc_output="$(printf '%s\n' "${app_doc}" | "${classifier}")"
  assert_output_line "${doc_output}" 'docs_only=true'
  assert_output_line "${doc_output}" 'api=false'
  assert_output_line "${doc_output}" 'web=false'
done
assert_output_line "${services_output}" 'api_tests=true'
assert_output_line "${services_output}" 'api_sqlc=false'
assert_output_line "${services_output}" 'api_migrations=false'
assert_output_line "${services_output}" 'api_build=true'
assert_output_line "${services_output}" 'api_images=false'
assert_output_line "${queries_output}" 'api_sqlc=true'
assert_output_line "${queries_output}" 'api_migrations=false'
assert_output_line "${schema_output}" 'api_migrations=true'
assert_output_line "${schema_output}" 'api_sqlc=true'
test_output="$(printf '%s\n' apps/api/internal/tasking/service_test.go | "${classifier}")"
assert_output_line "${test_output}" 'api_tests=true'
assert_output_line "${test_output}" 'api_build=false'
assert_output_line "${test_output}" 'api_images=false'
assert_output_line "${test_output}" 'browser_smoke=false'
for action in deploy-production setup-grype; do
  action_output="$(printf '%s\n' ".github/actions/${action}/action.yml" | "${classifier}")"
  assert_output_line "${action_output}" 'ops=true'
  assert_output_line "${action_output}" 'api=false'
  assert_output_line "${action_output}" 'web=false'
  assert_output_line "${action_output}" 'browser_smoke=false'
done
go_action_output="$(printf '%s\n' .github/actions/setup-go/action.yml | "${classifier}")"
assert_output_line "${go_action_output}" 'api=true'
assert_output_line "${go_action_output}" 'web=false'
assert_output_line "${go_action_output}" 'migration_integration=true'
for action in setup-node npm-ci; do
  node_action_output="$(printf '%s\n' ".github/actions/${action}/action.yml" | "${classifier}")"
  assert_output_line "${node_action_output}" 'api=false'
  assert_output_line "${node_action_output}" 'web=true'
  assert_output_line "${node_action_output}" 'auth=true'
  assert_output_line "${node_action_output}" 'realtime=true'
  assert_output_line "${node_action_output}" 'migration_integration=true'
done
new_action_output="$(printf '%s\n' .github/actions/new-action/action.yml | "${classifier}")"
assert_output_line "${new_action_output}" 'api=true'
assert_output_line "${new_action_output}" 'api_migrations=true'
assert_output_line "${new_action_output}" 'web=true'
cross_contract_output="$(printf '%s\n' apps/web/src/core/contracts/generationContract.ts | "${classifier}")"
assert_output_line "${cross_contract_output}" 'api=true'
assert_output_line "${cross_contract_output}" 'api_tests=true'
assert_output_line "${cross_contract_output}" 'web=true'
assert_output_line "${cross_contract_output}" 'api_migrations=false'
for cross_path in \
  apps/web/src/api/workspaceTaskContract.ts \
  apps/web/src/features/admin/shared/core/taskMetricTokens.ts \
  contracts/library/library-source.fixtures.json \
  apps/api/internal/boards/previewscene/testdata/mixed.json; do
  owner_output="$(printf '%s\n' "${cross_path}" | "${classifier}")"
  assert_output_line "${owner_output}" 'web=true'
  assert_output_line "${owner_output}" 'api=true'
  assert_output_line "${owner_output}" 'api_tests=true'
  assert_output_line "${owner_output}" 'api_migrations=false'
done
echo 'CI change classifier contract tests passed'
