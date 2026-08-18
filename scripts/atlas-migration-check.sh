#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
default_repo_root="$(cd "${script_dir}/.." && pwd)"
repo_root="${ATLAS_REPOSITORY_ROOT:-${default_repo_root}}"
atlas_binary="${ATLAS_BINARY:-atlas}"
migrations_dir="${ATLAS_MIGRATIONS_DIR:-${repo_root}/apps/api/internal/db/migrations}"
dev_url="${ATLAS_DEV_URL:-}"
base_sha="${ATLAS_BASE_SHA:-}"
head_sha="${ATLAS_HEAD_SHA:-HEAD}"

fail() {
  printf 'atlas-migration-check: %s\n' "$1" >&2
  exit 1
}

[[ -d "${migrations_dir}" ]] || fail "migration directory does not exist: ${migrations_dir}"
[[ -f "${migrations_dir}/atlas.sum" ]] || fail "atlas.sum is missing from ${migrations_dir}"
[[ -n "${dev_url}" ]] || fail 'ATLAS_DEV_URL is required for Fresh migration validation'

resolve_added_migrations() {
  local migration_prefix="${migrations_dir#"${repo_root}"}"
  if [[ -n "${base_sha}" ]]; then
    git -C "${repo_root}" rev-parse --verify "${base_sha}^{commit}" >/dev/null 2>&1 || \
      fail "ATLAS_BASE_SHA is not a commit: ${base_sha}"
    git -C "${repo_root}" diff --diff-filter=A --name-only "${base_sha}" "${head_sha}" -- \
      "${migration_prefix#/}"'/*.sql' || true
  else
    git -C "${repo_root}" diff --diff-filter=A --name-only "${head_sha}" -- \
      "${migration_prefix#/}"'/*.sql' || true
  fi
  git -C "${repo_root}" ls-files --others --exclude-standard -- \
    "${migration_prefix#/}"'/*.sql' || true
}

destructive_pattern='^[[:space:]]*(DROP[[:space:]]+(TABLE|COLUMN|SCHEMA|INDEX|TYPE|VIEW|MATERIALIZED[[:space:]]+VIEW|SEQUENCE|FUNCTION|TRIGGER)([[:space:]]|;|$)|TRUNCATE([[:space:]]|;|$)|ALTER[[:space:]]+TABLE[[:space:]].*[[:space:]]DROP([[:space:]]|;|$))'

check_destructive_changes() {
  local relative_path path
  local -a migrations=()
  while IFS= read -r relative_path; do
    [[ -n "${relative_path}" ]] && migrations+=("${relative_path}")
  done < <(resolve_added_migrations | sort -u)

  ((${#migrations[@]} > 0)) || return 0

  for relative_path in "${migrations[@]}"; do
    path="${repo_root}/${relative_path}"
    [[ -f "${path}" ]] || fail "new migration disappeared: ${relative_path}"
    if grep -nE "${destructive_pattern}" "${path}" >/dev/null; then
      if [[ "${ATLAS_ALLOW_DESTRUCTIVE:-}" != "1" ]] || ! grep -Eiq \
        '^[[:space:]]*--[[:space:]]*destructive:[[:space:]]*approved([[:space:]]|$)' "${path}"; then
        printf 'atlas-migration-check: destructive SQL is blocked in %s\n' "${relative_path}" >&2
        grep -nE "${destructive_pattern}" "${path}" >&2 || true
        printf '%s\n' 'Add "-- destructive: approved" and set ATLAS_ALLOW_DESTRUCTIVE=1 only after explicit review.' >&2
        exit 1
      fi
    fi
  done
}

check_destructive_changes

"${atlas_binary}" migrate validate \
  --dir "file://${migrations_dir}" \
  --dev-url "${dev_url}"
