#!/usr/bin/env bash

set -euo pipefail

web=false
api=false
auth=false
docker=false
ops=false
security=false
has_non_docs_change=false

mark_all_contexts() {
  web=true
  api=true
  auth=true
  docker=true
  ops=true
  security=true
}

while IFS= read -r changed_path; do
  [[ -n "${changed_path}" ]] || continue

  case "${changed_path}" in
    .github/workflows/* | .github/actions/* | .node-version | scripts/ci-change-classifier*)
      mark_all_contexts
      has_non_docs_change=true
      ;;
    apps/api/openapi.yaml)
      web=true
      api=true
      has_non_docs_change=true
      ;;
    docker/*)
      docker=true
      ops=true
      security=true
      has_non_docs_change=true
      ;;
    apps/web/package.json | apps/web/package-lock.json)
      web=true
      security=true
      has_non_docs_change=true
      ;;
    apps/auth/package.json | apps/auth/package-lock.json)
      auth=true
      security=true
      has_non_docs_change=true
      ;;
    apps/api/go.mod | apps/api/go.sum)
      api=true
      security=true
      has_non_docs_change=true
      ;;
    scripts/ci-parity.sh | scripts/ci-parity-test.sh)
      web=true
      api=true
      auth=true
      ops=true
      has_non_docs_change=true
      ;;
    apps/web/*)
      web=true
      has_non_docs_change=true
      ;;
    apps/api/Dockerfile)
      api=true
      docker=true
      has_non_docs_change=true
      ;;
    apps/auth/Dockerfile)
      auth=true
      docker=true
      has_non_docs_change=true
      ;;
    apps/api/*)
      api=true
      has_non_docs_change=true
      ;;
    apps/auth/*)
      auth=true
      has_non_docs_change=true
      ;;
    contracts/*)
      api=true
      auth=true
      has_non_docs_change=true
      ;;
    deploy/* | scripts/*)
      ops=true
      has_non_docs_change=true
      ;;
    README.md | docs/* | *.md)
      ;;
    *)
      mark_all_contexts
      has_non_docs_change=true
      ;;
  esac
done

printf 'web=%s\n' "${web}"
printf 'api=%s\n' "${api}"
printf 'auth=%s\n' "${auth}"
printf 'docker=%s\n' "${docker}"
printf 'ops=%s\n' "${ops}"
printf 'security=%s\n' "${security}"
printf 'docs_only=%s\n' "$([[ "${has_non_docs_change}" == false ]] && printf true || printf false)"
