#!/usr/bin/env bash
set -euo pipefail
web=false; api=false; auth=false; realtime=false; migration_integration=false; docker=false; ops=false
api_tests=false; api_sqlc=false; api_migrations=false; api_build=false; api_images=false; browser_smoke=false
web_audit=false; api_audit=false; auth_audit=false; realtime_audit=false
has_non_docs_change=false
mark_api() { api=true; api_tests=true; }
mark_api_all() { mark_api; api_sqlc=true; api_migrations=true; api_build=true; }
mark_node_consumers() { web=true; auth=true; realtime=true; migration_integration=true; ops=true; }
mark_all_contexts() { mark_api_all; web=true; auth=true; realtime=true; migration_integration=true; docker=true; api_images=true; ops=true; browser_smoke=true; web_audit=true; api_audit=true; auth_audit=true; realtime_audit=true; }
while IFS= read -r changed_path; do
  [[ -n "${changed_path}" ]] || continue
  case "${changed_path}" in
    # Verification tooling is exercised by Ops; it does not change app runtimes.
    .github/workflows/ops.yml | scripts/ci-change-classifier* | scripts/ci-workflow-contract* | scripts/ci-baseline-test.sh | scripts/ci-gate-test.sh) ops=true ;;
    .github/workflows/ci.yml | scripts/ci-baseline* | scripts/ci-gate*) mark_all_contexts ;;
    .github/actions/deploy-production/* | .github/actions/setup-grype/* | .github/actions/pipeline-metrics/*) ops=true ;;
    .github/actions/setup-go/*) mark_api_all; auth=true; migration_integration=true; ops=true ;;
    .github/actions/setup-node/* | .github/actions/npm-ci/* | .node-version) mark_node_consumers ;;
    .github/actions/*) mark_all_contexts ;;
    .github/workflows/web.yml) web=true; ops=true ;;
    .github/workflows/api.yml) mark_api_all; api_images=true; docker=true; ops=true ;;
    .github/workflows/auth.yml) auth=true; docker=true; ops=true ;;
    .github/workflows/realtime.yml) realtime=true; docker=true; ops=true ;;
    .github/workflows/migration-integration.yml) migration_integration=true; ops=true ;;
    .github/workflows/docker-images.yml | .github/workflows/deploy-production.yml) ops=true ;;
    scripts/ci-migration-integration* | scripts/atlas-migration-check*) mark_api_all; auth=true; migration_integration=true; ops=true ;;
    scripts/sql-workspace-scope-check* | scripts/openapi-pattern-check* | scripts/check-backend-copy*) mark_api; ops=true ;;
    scripts/ci-parity.sh | scripts/ci-parity-test.sh) mark_all_contexts ;;
    scripts/ci-api-* ) mark_api_all; ops=true ;;
    scripts/ci-web-*) web=true; ops=true ;;
    apps/*/CONTEXT.md | apps/*/README.md | apps/*/DESIGN.md) continue ;;
    # Go parity checks read these Web contracts without importing them.
    apps/web/src/core/contracts/imageServices.ts | apps/web/src/core/contracts/generationContract.ts | apps/web/src/features/canvas/core/execution/executionContract.ts | apps/web/src/features/admin/shared/core/serviceAdminMeta.ts | apps/web/src/api/realtime/workspaceEventStream.ts | apps/web/src/api/tasks/workspaceTaskContract.ts | apps/web/src/features/admin/shared/ui/styling/taskMetricTokens.ts) web=true; mark_api ;;
    apps/api/openapi.public.yaml | apps/api/openapi.control.yaml | apps/api/openapi.events.yaml) web=true; mark_api ;;
    apps/api/atlas.hcl | apps/api/internal/db/migrations/*.sql | apps/api/internal/db/migrations/atlas.sum | apps/api/internal/db/schema/*) mark_api_all; migration_integration=true ;;
    apps/api/cmd/migrator/* | apps/api/cmd/migrationhistory/* | apps/api/cmd/schemarelease/* | apps/api/internal/runtime/infrastructure/migration/* | apps/api/internal/runtime/composition/wiring/migration.go | apps/api/internal/db/migrationhistory/* | apps/api/internal/db/schemaassembly/* | apps/api/internal/db/schemacontract/* | apps/api/internal/db/schemarelease/*) mark_api_all; migration_integration=true ;;
    # Preview fixtures are also runtime inputs of Web renderer/gallery tests.
    apps/api/internal/boards/previewscene/testdata/*) web=true; mark_api ;;
    contracts/library/library-source.fixtures.json) web=true; mark_api; auth=true ;;
    apps/api/internal/db/queries/* | apps/api/internal/db/sqlc/* | apps/api/sqlc.yaml | apps/api/sqlc.yml) mark_api; api_sqlc=true ;;
    apps/api/internal/testdb/* | apps/api/internal/testsupport/* | apps/api/internal/db/*) mark_api_all ;;
    apps/auth/Dockerfile) auth=true; docker=true; migration_integration=true; browser_smoke=true; ops=true ;;
    apps/auth/src/migrate.ts | apps/auth/src/infrastructure/*[mM]igration*) auth=true; docker=true; migration_integration=true; browser_smoke=true ;;
    docker/postgres/* | docker/compose*.yml | docker/.env | docker/.env.example | docker/images.env.example | docker/migration.env.example) ops=true; migration_integration=true ;;
    apps/api/internal/platform/config/* | apps/api/.env.example) mark_api; api_build=true; ops=true; browser_smoke=true ;;
    apps/auth/src/infrastructure/env.ts | apps/auth/.env.example) auth=true; ops=true; browser_smoke=true ;;
    apps/realtime/src/infrastructure/env.ts | apps/realtime/.env.example) realtime=true; ops=true; browser_smoke=true ;;
    apps/api/install-atlas.sh | apps/api/atlas-checksums) mark_api_all; migration_integration=true; ops=true; api_images=true; docker=true ;;
    apps/api/Dockerfile | apps/api/.dockerignore) mark_api_all; api_images=true; docker=true; browser_smoke=true; ops=true ;;
    apps/realtime/Dockerfile) realtime=true; docker=true; browser_smoke=true; ops=true ;;
    apps/web/package.json | apps/web/package-lock.json) web=true; web_audit=true ;;
    apps/auth/package.json | apps/auth/package-lock.json) auth=true; auth_audit=true; docker=true; browser_smoke=true ;;
    apps/realtime/package.json | apps/realtime/package-lock.json) realtime=true; realtime_audit=true; docker=true; browser_smoke=true ;;
    apps/api/go.mod | apps/api/go.sum) auth=true; migration_integration=true; ops=true; mark_api_all; api_audit=true; api_images=true; docker=true; browser_smoke=true ;;
    apps/api/*_test.go) mark_api ;;
    apps/api/*.go) mark_api; api_build=true; browser_smoke=true ;;
    apps/web/*) web=true ;;
    apps/api/*) mark_api_all ;;
    apps/auth/*) auth=true; browser_smoke=true ;;
    apps/realtime/*) realtime=true; browser_smoke=true ;;
    contracts/*) mark_api; auth=true; browser_smoke=true ;;
    deploy/* | scripts/*) ops=true ;;
    README.md | docs/* | *.md) continue ;;
    *) mark_all_contexts ;;
  esac
  has_non_docs_change=true
done
printf '%s\n' \
  "web=${web}" \
  "api=${api}" \
  "auth=${auth}" \
  "realtime=${realtime}" \
  "migration_integration=${migration_integration}" \
  "docker=${docker}" \
  "ops=${ops}" \
  "api_tests=${api_tests}" \
  "api_sqlc=${api_sqlc}" \
  "api_migrations=${api_migrations}" \
  "api_build=${api_build}" \
  "api_images=${api_images}" \
  "browser_smoke=${browser_smoke}" \
  "web_audit=${web_audit}" \
  "api_audit=${api_audit}" \
  "auth_audit=${auth_audit}" \
  "realtime_audit=${realtime_audit}"
printf 'docs_only=%s\n' "$([[ "${has_non_docs_change}" == false ]] && printf true || printf false)"
