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

# The Go module is the only owner of Git discovery and history policy.
# Inspect before linting; enforce immutability after risky SQL diagnostics.
history() {
  (cd "${default_repo_root}/apps/api" && go run ./cmd/migrationhistory \
    --repository "${repo_root}" --migrations "${migrations_dir}" \
    --base "${base_sha}" --head "${head_sha}" "$@")
}
added_migrations="$(history --list-changed)"

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

# History decisions are shared with schema release; the remaining checks
# enforce linear versions, current checksums and Fresh Atlas replay.
check_migration_conflicts() {
  history
  local migration_prefix="${migrations_dir#"${repo_root}"}"
  local relative_prefix="${migration_prefix#/}"
  local rest
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
