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
assert_output_line "${docs_output}" 'docker=false'
assert_output_line "${docs_output}" 'ops=false'
assert_output_line "${docs_output}" 'schema=false'
assert_output_line "${docs_output}" 'migration_integration=false'
assert_output_line "${docs_output}" 'docs_only=true'

web_output="$(printf '%s\n' apps/web/src/main.ts | "${classifier}")"

assert_output_line "${web_output}" 'web=true'
assert_output_line "${web_output}" 'api=false'
assert_output_line "${web_output}" 'auth=false'
assert_output_line "${web_output}" 'docker=false'
assert_output_line "${web_output}" 'docs_only=false'

services_output="$(printf '%s\n' apps/api/internal/tasking/service.go apps/auth/src/index.ts | "${classifier}")"

assert_output_line "${services_output}" 'api=true'
assert_output_line "${services_output}" 'auth=true'
assert_output_line "${services_output}" 'docker=false'
assert_output_line "${services_output}" 'web=false'
assert_output_line "${services_output}" 'docs_only=false'

ops_output="$(printf '%s\n' deploy/nginx/default.conf scripts/deploy-stack.sh | "${classifier}")"

assert_output_line "${ops_output}" 'ops=true'
assert_output_line "${ops_output}" 'web=false'
assert_output_line "${ops_output}" 'api=false'
assert_output_line "${ops_output}" 'auth=false'
assert_output_line "${ops_output}" 'docker=false'
assert_output_line "${ops_output}" 'docs_only=false'

ci_output="$(printf '%s\n' .github/workflows/ci.yml | "${classifier}")"

assert_output_line "${ci_output}" 'web=true'
assert_output_line "${ci_output}" 'api=true'
assert_output_line "${ci_output}" 'auth=true'
assert_output_line "${ci_output}" 'docker=true'
assert_output_line "${ci_output}" 'ops=true'
assert_output_line "${ci_output}" 'schema=true'
assert_output_line "${ci_output}" 'migration_integration=true'
assert_output_line "${ci_output}" 'docs_only=false'

openapi_output="$(printf '%s\n' apps/api/openapi.yaml | "${classifier}")"

assert_output_line "${openapi_output}" 'web=true'
assert_output_line "${openapi_output}" 'api=true'
assert_output_line "${openapi_output}" 'auth=false'

docker_output="$(printf '%s\n' docker/compose.yml | "${classifier}")"

assert_output_line "${docker_output}" 'api=false'
assert_output_line "${docker_output}" 'auth=false'
assert_output_line "${docker_output}" 'docker=false'
assert_output_line "${docker_output}" 'ops=true'
assert_output_line "${docker_output}" 'migration_integration=true'

dependency_output="$(printf '%s\n' apps/web/package-lock.json apps/api/go.sum | "${classifier}")"

assert_output_line "${dependency_output}" 'web=true'
assert_output_line "${dependency_output}" 'api=true'
assert_output_line "${dependency_output}" 'auth=false'
assert_output_line "${dependency_output}" 'docker=true'
assert_output_line "${dependency_output}" 'schema=false'

parity_output="$(printf '%s\n' scripts/ci-parity.sh | "${classifier}")"

assert_output_line "${parity_output}" 'web=true'
assert_output_line "${parity_output}" 'api=true'
assert_output_line "${parity_output}" 'auth=true'
assert_output_line "${parity_output}" 'docker=false'
assert_output_line "${parity_output}" 'ops=true'

migration_integration_output="$(printf '%s\n' scripts/ci-migration-integration.sh | "${classifier}")"

assert_output_line "${migration_integration_output}" 'web=false'
assert_output_line "${migration_integration_output}" 'api=true'
assert_output_line "${migration_integration_output}" 'auth=true'
assert_output_line "${migration_integration_output}" 'migration_integration=true'
assert_output_line "${migration_integration_output}" 'ops=true'

atlas_migration_output="$(printf '%s\n' scripts/atlas-migration-check.sh | "${classifier}")"

assert_output_line "${atlas_migration_output}" 'api=true'
assert_output_line "${atlas_migration_output}" 'auth=true'
assert_output_line "${atlas_migration_output}" 'migration_integration=true'
assert_output_line "${atlas_migration_output}" 'ops=true'

gate_output="$(printf '%s\n' scripts/ci-gate.sh | "${classifier}")"

assert_output_line "${gate_output}" 'web=true'
assert_output_line "${gate_output}" 'api=true'
assert_output_line "${gate_output}" 'auth=true'
assert_output_line "${gate_output}" 'schema=true'
assert_output_line "${gate_output}" 'migration_integration=true'
assert_output_line "${gate_output}" 'docker=true'
assert_output_line "${gate_output}" 'ops=true'

runtime_output="$(printf '%s\n' .node-version | "${classifier}")"

assert_output_line "${runtime_output}" 'web=true'
assert_output_line "${runtime_output}" 'api=true'
assert_output_line "${runtime_output}" 'auth=true'
assert_output_line "${runtime_output}" 'docker=true'
assert_output_line "${runtime_output}" 'ops=true'
assert_output_line "${runtime_output}" 'schema=true'
assert_output_line "${runtime_output}" 'migration_integration=true'

api_dockerfile_output="$(printf '%s\n' apps/api/Dockerfile | "${classifier}")"

assert_output_line "${api_dockerfile_output}" 'api=true'
assert_output_line "${api_dockerfile_output}" 'auth=false'
assert_output_line "${api_dockerfile_output}" 'docker=true'

unknown_output="$(printf '%s\n' tooling/new-config.toml | "${classifier}")"

assert_output_line "${unknown_output}" 'web=true'
assert_output_line "${unknown_output}" 'api=true'
assert_output_line "${unknown_output}" 'auth=true'
assert_output_line "${unknown_output}" 'docker=true'
assert_output_line "${unknown_output}" 'ops=true'
assert_output_line "${unknown_output}" 'docs_only=false'

echo 'CI change classifier contract tests passed'
