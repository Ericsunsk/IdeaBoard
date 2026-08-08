#!/usr/bin/env bash

set -euo pipefail

base_sha="${BASE_SHA:-}"
head_sha="${HEAD_SHA:-HEAD}"
[[ -n "${base_sha}" ]] || {
  printf 'migration history check: BASE_SHA is required\n' >&2
  exit 2
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

changed=()
while IFS= read -r entry; do
  [[ -n "${entry}" ]] && changed+=("${entry}")
done < <(
  git diff --no-renames --name-status "${base_sha}" "${head_sha}" -- 'apps/api/internal/db/migrations/*.sql'
)

baseline_added=0
added=0
legacy_deleted=0
invalid_change=0
for entry in "${changed[@]-}"; do
  [[ -n "${entry}" ]] || continue
  status="${entry%%$'\t'*}"
  path="${entry#*$'\t'}"
  case "${status}" in
    A)
	  ((added += 1))
      [[ "${path}" == *"_baseline.sql" ]] && ((baseline_added += 1))
      ;;
    D|R*)
      ((legacy_deleted += 1))
      ;;
    M)
      ((invalid_change += 1))
      ;;
    *)
      ((invalid_change += 1))
      ;;
  esac
done

if (( baseline_added > 1 || invalid_change > 0 || (legacy_deleted > 0 && (baseline_added != 1 || added != 1)) )); then
  printf 'released migration history is immutable; use one explicit clean-break baseline to replace legacy files\n' >&2
  exit 1
fi
printf 'migration history immutability check passed\n'
