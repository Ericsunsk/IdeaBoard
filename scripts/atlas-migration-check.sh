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
  local relative_glob="${migration_prefix#/}"'/*.sql'
  # NOTE: --diff-filter includes R so a staged rename (git reports status R,
  # not A) still surfaces its new path for the destructive/type/NOT-NULL
  # lint. --name-status is required because --name-only hides the rename
  # target distinction; field 3 is the new path for R, field 2 otherwise.
  if [[ -n "${base_sha}" ]]; then
    git -C "${repo_root}" diff --diff-filter=AMR --name-status "${base_sha}" "${head_sha}" -- \
      "${relative_glob}" | awk -F'\t' '{ print ($1 ~ /^R/ ? $3 : $2) }' || true
  fi
  git -C "${repo_root}" diff --diff-filter=AMR --name-status "${head_sha}" -- \
    "${relative_glob}" | awk -F'\t' '{ print ($1 ~ /^R/ ? $3 : $2) }' || true
  git -C "${repo_root}" ls-files --others --exclude-standard -- \
    "${relative_glob}" || true
}

# Resolve one consistent added-file set for every gate.
if [[ -n "${base_sha}" ]]; then
  git -C "${repo_root}" rev-parse --verify "${base_sha}^{commit}" >/dev/null 2>&1 || \
    fail "ATLAS_BASE_SHA is not a commit: ${base_sha}"
fi
added_migrations="$(resolve_added_migrations | sort -u)"
# A file added between base and head but renamed in the worktree appears
# under both its old and new names; the old name no longer exists on disk
# and is not a migration to lint. Drop rename sources so the freshness
# check below does not fail on a path the rename itself removed.
migration_prefix="${migrations_dir#"${repo_root}"}"
renamed_sources="$(git -C "${repo_root}" diff --name-status "${head_sha}" -- \
  "${migration_prefix#/}"'/*.sql' | awk -F'\t' '$1 ~ /^R/ { print $2 }' || true)"
if [[ -n "${renamed_sources}" ]]; then
  added_migrations="$(printf '%s\n' "${added_migrations}" | grep -Fxv -f <(printf '%s\n' "${renamed_sources}") || true)"
fi
while IFS= read -r relative_path; do
  [[ -n "${relative_path}" ]] || continue
  [[ -f "${repo_root}/${relative_path}" ]] || fail "new migration disappeared: ${relative_path}"
done <<<"${added_migrations}"

# Destructive DDL that must be reviewed before it reaches a database. The
# object-type list is shared by the bare `DROP <object>` form and the
# `ALTER TABLE ... DROP <object>` form so both spellings of the same change are
# caught. CONSTRAINT is included because dropping a foreign key or a CHECK
# silently removes referential integrity or a validation rule, which a reviewer
# scanning for DROP TABLE would not notice.
#
# Community lint gate (Atlas Pro `migrate lint` is intentionally NOT a
# dependency here; DEVELOPMENT.md keeps `migrate validate` as the CI
# contract). The risky patterns below cover the lint-equivalent fail
# conditions: destructive/DROP, column type changes, and ADD/SET NOT NULL
# without a DEFAULT. They run before the `--dev-url` Fresh replay at the
# bottom of this script.
destructive_pattern='^[[:space:]]*(DROP[[:space:]]+(TABLE|COLUMN|CONSTRAINT|SCHEMA|INDEX|TYPE|VIEW|MATERIALIZED[[:space:]]+VIEW|SEQUENCE|FUNCTION|TRIGGER)([[:space:]]|;|$)|TRUNCATE([[:space:]]|;|$)|ALTER[[:space:]]+TABLE[[:space:]].*[[:space:]]DROP[[:space:]]+(TABLE|COLUMN|CONSTRAINT|SCHEMA|INDEX|TYPE|VIEW|MATERIALIZED[[:space:]]+VIEW|SEQUENCE|FUNCTION|TRIGGER)([[:space:]]|;|$))'

# Column type changes require a table/index rewrite and can fail on existing
# data (PostgreSQL PG301 family). There is no approved bypass: rewrite the
# migration (expand-and-contract) instead.
type_change_pattern='ALTER[[:space:]]+TABLE[[:space:]]+.*ALTER[[:space:]]+(COLUMN[[:space:]]+)?[^[:space:];]+[[:space:]]+(SET[[:space:]]+DATA[[:space:]]+)?TYPE([[:space:]]|;|$)'

# Promoting a column to NOT NULL scans the table and fails on existing NULLs
# (PostgreSQL PG303 / MF104 family). There is no approved bypass: backfill
# first or keep the column nullable.
set_not_null_pattern='ALTER[[:space:]]+TABLE[[:space:]]+.*ALTER[[:space:]]+(COLUMN[[:space:]]+)?[^[:space:];]+[[:space:]]+SET[[:space:]]+NOT[[:space:]]+NULL'

# Flatten statements and separate top-level ALTER actions with tabs. Quoted
# contents and comments are masked so their commas, semicolons and keywords
# cannot hide or invent a DDL action. This is a lexical boundary scan only;
# Atlas remains responsible for SQL syntax and semantic validation.
normalized_statements() {
  awk '
    BEGIN { apostrophe = sprintf("%c", 39) }
    {
      for (i = 1; i <= length($0); i++) {
        c = substr($0, i, 1)
        next_c = substr($0, i + 1, 1)
        if (block_depth) {
          if (c == "/" && next_c == "*") { block_depth++; i++ }
          else if (c == "*" && next_c == "/") { block_depth--; i++ }
          continue
        }
        if (dollar_quote != "") {
          if (substr($0, i, length(dollar_quote)) == dollar_quote) {
            i += length(dollar_quote) - 1
            dollar_quote = ""
          }
          continue
        }
        if (quote != "") {
          if (c == quote) {
            if (next_c == quote) i++
            else quote = ""
          }
          continue
        }
        if (c == "-" && next_c == "-") break
        if (c == "/" && next_c == "*") { block_depth++; i++; sql = sql " "; continue }
        if (c == apostrophe || c == "\"") { quote = c; sql = sql " quoted "; continue }
        if (c == "$" && match(substr($0, i), /^\$([A-Za-z_][A-Za-z_0-9]*)?\$/)) {
          dollar_quote = substr($0, i, RLENGTH)
          i += RLENGTH - 1
          sql = sql " quoted "
          continue
        }
        if (c == "(") depth++
        if (c == ")") depth--
        if (c == ";") { print sql; sql = ""; continue }
        if (c == "," && depth == 0) sql = sql "\t"
        else if (c ~ /[[:space:]]/) sql = sql " "
        else sql = sql toupper(c)
      }
      sql = sql " "
    }
    END { if (sql ~ /[^[:space:]]/) print sql }
  ' "$1"
}

check_risky_changes() {
  local relative_path path stmt action
  local -a actions=()
  while IFS= read -r relative_path; do
    [[ -n "${relative_path}" ]] || continue
    path="${repo_root}/${relative_path}"
    while IFS= read -r stmt; do
      if [[ "${stmt}" =~ ${destructive_pattern} ]]; then
        if [[ "${ATLAS_ALLOW_DESTRUCTIVE:-}" != "1" ]] || ! grep -Eiq \
          '^[[:space:]]*--[[:space:]]*destructive:[[:space:]]*approved([[:space:]]|$)' "${path}"; then
          printf 'atlas-migration-check: destructive SQL is blocked in %s\n' "${relative_path}" >&2
          printf 'blocked statement: %s\n' "${stmt}" >&2
          printf '%s\n' 'Add "-- destructive: approved" and set ATLAS_ALLOW_DESTRUCTIVE=1 only after explicit review.' >&2
          exit 1
        fi
      fi
      if [[ "${stmt}" =~ ${type_change_pattern} ]]; then
        printf 'atlas-migration-check: column type change is blocked in %s\n' "${relative_path}" >&2
        printf 'blocked statement: %s\n' "${stmt}" >&2
        printf '%s\n' 'Use expand-and-contract instead; this gate fails closed with no approved bypass.' >&2
        exit 1
      fi
      if [[ "${stmt}" =~ ${set_not_null_pattern} ]]; then
        printf 'atlas-migration-check: ALTER COLUMN ... SET NOT NULL is blocked in %s\n' "${relative_path}" >&2
        printf 'blocked statement: %s\n' "${stmt}" >&2
        printf '%s\n' 'Backfill NULLs first; this gate fails closed with no approved bypass.' >&2
        exit 1
      fi
      # Inspect ADD COLUMN independently: another action with a DEFAULT or
      # ADD CONSTRAINT must not exempt a new NOT NULL column without a default.
      [[ "${stmt}" =~ ^[[:space:]]*ALTER[[:space:]]+TABLE[[:space:]] ]] || continue
      IFS=$'\t' read -r -a actions <<<"${stmt}"
      for action in "${actions[@]}"; do
        [[ "${action}" =~ ADD[[:space:]]+ ]] || continue
        [[ "${action}" =~ ADD[[:space:]]+CONSTRAINT[[:space:]] ]] && continue
        [[ "${action}" =~ NOT[[:space:]]+NULL ]] || continue
        [[ "${action}" =~ (^|[[:space:]])DEFAULT([[:space:]]|$) ]] && continue
        printf 'atlas-migration-check: ADD COLUMN ... NOT NULL without DEFAULT is blocked in %s\n' "${relative_path}" >&2
        printf 'blocked action: %s\n' "${action}" >&2
        printf '%s\n' 'Backfill existing rows or add a DEFAULT; this gate fails closed with no approved bypass.' >&2
        exit 1
      done
    done < <(normalized_statements "${path}")
  done <<<"${added_migrations}"
}

check_risky_changes

# Clean-break rebaseline exemption for the history (M/D) gate only. Mirrors
# schemarelease.isCleanBreakRebaseline: exactly one added *_baseline.sql with
# a higher version than the single deleted *_baseline.sql, no M/R entries and
# no non-baseline additions. Deletions of the squashed non-baseline chain are
# expected and allowed. Anything else fails closed (no exemption). The
# destructive/type/NOT-NULL lint, duplicate/base-max/atlas.sum checks, and
# `migrate validate` below are NOT exempted.
baseline_version_of() {
  local name version
  name="$(basename "$1")"
  case "${name}" in
    *_baseline.sql) version="${name%_baseline.sql}" ;;
    *) return 1 ;;
  esac
  [[ "${version}" =~ ^[0-9]{14}$ ]] || return 1
  printf '%s' "${version}"
}

is_clean_break_rebaseline() {
  local migration_prefix="${migrations_dir#"${repo_root}"}"
  local relative_prefix="${migration_prefix#/}"
  local raw status path version
  local added_version="" deleted_version=""
  local added_count=0 deleted_count=0
  if [[ -n "${base_sha}" ]]; then
    raw="$( { git -C "${repo_root}" diff --no-renames --name-status "${base_sha}" "${head_sha}" -- "${relative_prefix}/"
      git -C "${repo_root}" diff --no-renames --name-status "${head_sha}" -- "${relative_prefix}/"; } || true)"
  else
    raw="$(git -C "${repo_root}" diff --no-renames --name-status "${head_sha}" -- "${relative_prefix}/" || true)"
  fi
  # Untracked migration files are invisible to `git diff`; count them as
  # additions so a stray new migration cannot hide behind the exemption.
  raw="${raw}"$'\n'"$(git -C "${repo_root}" ls-files --others --exclude-standard -- "${relative_prefix}/*.sql" 2>/dev/null | awk '{print "A\t" $0}' || true)"
  while IFS= read -r line; do
    [[ -n "${line}" ]] || continue
    status="${line%%$'\t'*}"
    path="${line#*$'\t'}"
    case "${path}" in
      "${relative_prefix}/atlas.sum") continue ;;
    esac
    case "${path}" in
      *.sql) ;;
      *) continue ;;
    esac
    case "${status}" in
      A)
        version="$(baseline_version_of "${path}")" || return 1
        [[ "${added_count}" -eq 0 ]] || return 1
        added_version="${version}"
        added_count=1
        ;;
      D)
        if version="$(baseline_version_of "${path}")"; then
          [[ "${deleted_count}" -eq 0 ]] || return 1
          deleted_version="${version}"
          deleted_count=1
        fi
        ;;
      *) return 1 ;;
    esac
  done <<<"${raw}"
  [[ "${added_count}" -eq 1 && "${deleted_count}" -eq 1 ]] || return 1
  [[ 10#"${added_version}" -gt 10#"${deleted_version}" ]] || return 1
  return 0
}

# Conflict detection: migration history is linear. Edited/deleted *.sql files,
# duplicate version prefixes, versions that do not sort after the base max
# (stale branch, needs rebase), and added files missing from atlas.sum
# (forgot `atlas migrate hash`) all fail here. atlas.sum itself is expected
# to change on every new migration, so only *.sql edits are rejected.
check_migration_conflicts() {
  local migration_prefix="${migrations_dir#"${repo_root}"}"
  local relative_prefix="${migration_prefix#/}"
  local changed entry rest
  if [[ -n "${base_sha}" ]]; then
    # Commit-range diff covers CI (clean checkout at head); worktree diff
    # covers local runs with uncommitted edits. Untracked files are handled
    # by resolve_added_migrations, not here.
    changed="$( { git -C "${repo_root}" diff --diff-filter=MD --name-only "${base_sha}" "${head_sha}" -- "${relative_prefix}/"
      git -C "${repo_root}" diff --diff-filter=MD --name-only "${head_sha}" -- "${relative_prefix}/"; } || true)"
  else
    changed="$(git -C "${repo_root}" diff --diff-filter=MD --name-only "${head_sha}" -- "${relative_prefix}/" || true)"
  fi
  if is_clean_break_rebaseline; then
    printf 'atlas-migration-check: clean-break rebaseline detected, skipping history immutability gate\n' >&2
  else
  while IFS= read -r entry; do
    [[ -n "${entry}" ]] || continue
    case "${entry}" in
      "${relative_prefix}/atlas.sum") continue ;;
      "${relative_prefix}/"*.sql)
        printf 'atlas-migration-check: migration history is immutable, %s was modified or deleted\n' "${entry}" >&2
        printf '%s\n' 'Add a new higher-version migration instead of editing history.' >&2
        exit 1
        ;;
    esac
  done <<<"${changed}"
  fi

  local filename seen_versions duplicate
  seen_versions="$(for filename in "${migrations_dir}"/*.sql; do
    [[ -e "${filename}" ]] || continue
    rest="$(basename "${filename}")"
    printf '%s\n' "${rest%%_*}"
  done | sort)"
  duplicate="$(printf '%s\n' "${seen_versions}" | uniq -d || true)"
  if [[ -n "${duplicate}" ]]; then
    printf 'atlas-migration-check: duplicate migration version(s): %s\n' "${duplicate}" >&2
    exit 1
  fi

  local relative_path basename_file added_version base_entry base_max base_version
  if [[ -n "${base_sha}" ]]; then
    base_max=""
    while IFS= read -r base_entry; do
      [[ -n "${base_entry}" ]] || continue
      rest="$(basename "${base_entry}")"
      case "${rest}" in
        *_*.sql)
          base_version="${rest%%_*}"
          [[ "${base_version}" =~ ^[0-9]+$ ]] || continue
          if [[ -z "${base_max}" ]] || [[ "${base_version}" > "${base_max}" ]]; then
            base_max="${base_version}"
          fi
          ;;
      esac
    done < <(git -C "${repo_root}" ls-tree -r --name-only "${base_sha}" -- "${relative_prefix}/" || true)
    if [[ -n "${base_max}" ]]; then
      while IFS= read -r relative_path; do
        [[ -n "${relative_path}" ]] || continue
        basename_file="$(basename "${relative_path}")"
        added_version="${basename_file%%_*}"
        [[ "${added_version}" =~ ^[0-9]+$ ]] || fail "unexpected migration filename: ${basename_file}"
        if [[ ! "${added_version}" > "${base_max}" ]]; then
          printf 'atlas-migration-check: new migration %s does not sort after base max %s; rebase onto the latest main\n' "${basename_file}" "${base_max}" >&2
          exit 1
        fi
      done <<<"${added_migrations}"
    fi
  fi

  while IFS= read -r relative_path; do
    [[ -n "${relative_path}" ]] || continue
    basename_file="$(basename "${relative_path}")"
    grep -Fq "${basename_file} " "${migrations_dir}/atlas.sum" || \
      fail "atlas.sum is missing entry for ${basename_file}; run 'atlas migrate hash'"
  done <<<"${added_migrations}"
}

check_migration_conflicts

"${atlas_binary}" migrate validate \
  --dir "file://${migrations_dir}" \
  --dev-url "${dev_url}"
