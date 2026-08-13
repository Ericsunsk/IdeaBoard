#!/usr/bin/env bash
set -euo pipefail
web=false; api=false; auth=false; schema=false; migration_integration=false; docker=false; ops=false
web_audit=false; api_audit=false; auth_audit=false
has_non_docs_change=false
mark_all_contexts() { web=true; api=true; auth=true; schema=true; migration_integration=true; docker=true; ops=true; web_audit=true; api_audit=true; auth_audit=true; }
while IFS= read -r changed_path; do
  [[ -n "${changed_path}" ]] || continue
  case "${changed_path}" in
    .github/workflows/* | .github/actions/* | .node-version | scripts/ci-change-classifier* | scripts/ci-gate*) mark_all_contexts; has_non_docs_change=true ;;
    scripts/ci-migration-integration*) api=true; auth=true; migration_integration=true; ops=true; has_non_docs_change=true ;;
    apps/api/openapi.yaml) web=true; api=true; has_non_docs_change=true ;;
    apps/api/internal/db/migrations/* | apps/api/internal/db/schema/*) api=true; schema=true; migration_integration=true; has_non_docs_change=true ;;
    apps/api/cmd/schema-release/* | apps/api/internal/db/schemarelease/* | apps/api/internal/db/schemaassembly/*) api=true; schema=true; has_non_docs_change=true ;;
    apps/auth/src/migrate.ts | apps/auth/src/infrastructure/*migration* | apps/auth/Dockerfile) auth=true; docker=true; migration_integration=true; has_non_docs_change=true ;;
    docker/postgres/* | docker/compose*.yml | docker/migration.env.example) ops=true; migration_integration=true; has_non_docs_change=true ;;
    apps/api/Dockerfile) api=true; docker=true; has_non_docs_change=true ;;
    apps/web/package.json | apps/web/package-lock.json) web=true; web_audit=true; has_non_docs_change=true ;;
    apps/auth/package.json | apps/auth/package-lock.json) auth=true; auth_audit=true; docker=true; has_non_docs_change=true ;;
    apps/api/go.mod | apps/api/go.sum) api=true; api_audit=true; docker=true; has_non_docs_change=true ;;
    scripts/ci-parity.sh | scripts/ci-parity-test.sh) web=true; api=true; auth=true; ops=true; has_non_docs_change=true ;;
    apps/web/*) web=true; has_non_docs_change=true ;;
    apps/api/*) api=true; has_non_docs_change=true ;;
    apps/auth/*) auth=true; has_non_docs_change=true ;;
    contracts/*) api=true; auth=true; has_non_docs_change=true ;;
    deploy/* | scripts/*) ops=true; has_non_docs_change=true ;;
    README.md | docs/* | *.md) ;;
    *) mark_all_contexts; has_non_docs_change=true ;;
  esac
done
printf 'web=%s\n' "${web}"
printf 'api=%s\n' "${api}"
printf 'auth=%s\n' "${auth}"
printf 'schema=%s\n' "${schema}"
printf 'migration_integration=%s\n' "${migration_integration}"
printf 'docker=%s\n' "${docker}"
printf 'ops=%s\n' "${ops}"
printf 'web_audit=%s\n' "${web_audit}"
printf 'api_audit=%s\n' "${api_audit}"
printf 'auth_audit=%s\n' "${auth_audit}"
printf 'docs_only=%s\n' "$([[ "${has_non_docs_change}" == false ]] && printf true || printf false)"
