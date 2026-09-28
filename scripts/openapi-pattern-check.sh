#!/usr/bin/env bash
# openapi-pattern-check.sh
#
# OpenAPI scalar-pattern drift guard.
#
# The three live contract specs hand-repeat the same scalar ID patterns
# (ws_/task_/usr_/... full-match regexes) in apps/api/openapi.{public,
# control,events}.yaml. One drifted regex is a 400/404 shape divergence or a
# validation bypass, and oasdiff only diffs base-vs-head of the same file, so
# it cannot catch cross-spec drift. The smallest change that kills the drift
# class is this grep guard: every anchored ID pattern must equal its canonical
# value, and any other pattern attached to the same parameter/property name
# must agree wherever that name is shared.
#
# Rules:
#   Rule 1 (ID families): any pattern: value starting with ^<prefix>_ where
#     <prefix>_ is a known family below must equal the canonical regex for
#     that family, unless explicitly allowlisted with a reason.
#   Rule 2 (shared by name): any other pattern: value is keyed by its owner
#     (nearest schema/parameter/property name, normalized: lowercase, no
#     underscores, leading "control" stripped so ControlUserId and UserId
#     compare). One owner name with two distinct patterns fails.
#     Single-occurrence patterns pass (additive, oasdiff philosophy).
#   Rule 3 (operation contract): every operation carries an operationId, and
#     every mutation (POST/PUT/PATCH/DELETE) states its Idempotency-Key
#     requirement status in its own block — either the shared IdempotencyKey
#     parameter or an explicit description ("Requires ..." / "No
#     Idempotency-Key ..."). GET/HEAD reads are exempt from the idempotency
#     half. Grandfathered gaps live in idem_status_allowlist below.
#
# A missing live spec is a hard failure, never a silent skip (mirrors
# openapi-breaking-check.sh).
#
# To wire it, mirror the sibling contract lanes: invoke
# scripts/openapi-pattern-check-test.sh from .github/workflows/ops.yml
# ("Verify CI orchestration contracts"), and add both scripts to the
# bash -n / shellcheck lists in that file.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
specs=(
  "$repo_root/apps/api/openapi.public.yaml"
  "$repo_root/apps/api/openapi.control.yaml"
  "$repo_root/apps/api/openapi.events.yaml"
)

# Canonical full-match regex per ID prefix family, as observed across the
# three live specs. Single-quoted: the trailing $ must stay literal.
canonical_patterns=(
  'usr_|^usr_[A-Za-z0-9_-]+$'
  'org_|^org_[A-Za-z0-9_-]+$'
  'ws_|^ws_[A-Za-z0-9_-]+$'
  'board_|^board_[A-Za-z0-9_-]+$'
  'task_|^task_[A-Za-z0-9_-]+$'
  'asset_|^asset_[A-Za-z0-9_-]+$'
  'upload_|^upload_[A-Za-z0-9_-]+$'
  'evt_|^evt_[A-Za-z0-9_-]+$'
  'sub_|^sub_[A-Za-z0-9_-]+$'
  'sht_|^sht_[A-Za-z0-9_-]+$'
  'lfolder_|^lfolder_[A-Za-z0-9_-]+$'
  'lasset_|^lasset_[A-Za-z0-9_-]+$'
  'visq_|^visq_[A-Za-z0-9_-]+$'
)

# owner|pattern exceptions to Rule 1, with reasons. Keep minimal: adding a
# line here weakens the guard for that owner.
pattern_allowlist=(
  # BatchGetMediaAssetsRequest.asset_ids uses a loose prefix match, not full
  # ID validation (the kernel validates each id on read).
  'asset_ids|^asset_'
)

# "METHOD path operationId" exceptions to the Rule 3 idempotency-status
# half, with reasons. Empty since the workspace member grant/remove
# pair adopted the shared IdempotencyKey parameter (board parity):
# addWorkspaceMember and removeWorkspaceMember now require the key.
# New mutations must not be added here; state the requirement status
# in the operation block instead.
idem_status_allowlist=()

for spec in "${specs[@]}"; do
  if [[ ! -f "$spec" ]]; then
    echo "openapi-pattern-check: spec $spec is missing" >&2
    exit 1
  fi
done

workdir="$(mktemp -d)"
cleanup_path "$workdir"
rows="$workdir/rows.tsv"

# Extract file, line, normalized owner, owner, pattern per pattern: line.
# Owner = nearest non-structural ancestor key, unless a `name:` field was seen
# after that key was pushed (inline path/query params). Normalization lets
# ControlUserId (name: user_id), UserId, and events WorkspaceId compare.
awk '
  function is_structural(k) {
    return index(" schema properties items type content responses parameters components schemas headers paths ", " " k " ") > 0
  }
  function norm_owner(s,   t) {
    t = tolower(s)
    gsub(/_/, "", t)
    sub(/^control/, "", t)
    return t
  }
  /^[[:space:]]*(-[[:space:]]+)?name:[[:space:]]*[^[:space:]#]/ {
    v = $0
    sub(/^[^:]*:[[:space:]]*/, "", v)
    sub(/[[:space:]]+(#.*)?$/, "", v)
    last_name = v
    last_name_ln = NR
    next
  }
  /^[[:space:]]*[^[:space:]:][^:]*:[[:space:]]*(#.*)?$/ {
    line = $0
    sub(/^[[:space:]]*/, "", line)
    sub(/:[[:space:]]*(#.*)?$/, "", line)
    key = line
    match($0, /^[[:space:]]*/)
    ind = RLENGTH
    while (sp > 0 && stack_ind[sp] >= ind) {
      sp--
    }
    sp++
    stack_ind[sp] = ind
    stack_key[sp] = key
    stack_ln[sp] = NR
    next
  }
  /^[[:space:]]*pattern:[[:space:]]*[^[:space:]]/ {
    pat = $0
    sub(/^[^:]*:[[:space:]]*/, "", pat)
    sub(/[[:space:]]+$/, "", pat)
    if (pat ~ /^'"'"'.*'"'"'$/) {
      sub(/^'"'"'/, "", pat)
      sub(/'"'"'$/, "", pat)
    } else if (pat ~ /^".*"$/) {
      sub(/^"/, "", pat)
      sub(/"$/, "", pat)
    }
    owner = ""
    owner_ln = 0
    for (i = sp; i >= 1; i--) {
      if (!is_structural(stack_key[i])) {
        owner = stack_key[i]
        owner_ln = stack_ln[i]
        break
      }
    }
    if (last_name != "" && last_name_ln > owner_ln) {
      owner = last_name
    }
    if (owner == "") {
      owner = "(unknown)"
    }
    printf "%s\t%d\t%s\t%s\t%s\n", FILENAME, FNR, norm_owner(owner), owner, pat
  }
' "${specs[@]}" > "$rows"

fail=0
family_re='^\^([A-Za-z]+_)'

# check_family <prefix> <canonical>: rows whose pattern starts with
# ^<prefix>_ must equal canonical (or be allowlisted).
check_family() {
  local prefix="$1"
  local canonical="$2"
  local file line normowner owner pat key allowed entry
  while IFS="$(printf '\t')" read -r file line normowner owner pat; do
    [[ -n "$pat" ]] || continue
    case "$pat" in
      "^${prefix}"*)
        if [[ "$pat" != "$canonical" ]]; then
          key="${owner}|${pat}"
          allowed=0
          for entry in "${pattern_allowlist[@]}"; do
            if [[ "$key" == "$entry" ]]; then
              allowed=1
              break
            fi
          done
          if ((allowed == 0)); then
            echo "DIVERGENT-PATTERN $file:$line owner=$owner pattern=$pat (family $prefix expects $canonical)" >&2
            fail=1
          fi
        fi
        ;;
    esac
  done < "$rows"
  return 0
}

for entry in "${canonical_patterns[@]}"; do
  check_family "${entry%%|*}" "${entry#*|}"
done

# Rule 2: non-family patterns grouped by normalized owner name. One name with
# two distinct patterns fails; singletons pass.
nonfam="$workdir/nonfam.tsv"
fam="$workdir/fam.tsv"
: > "$nonfam"
: > "$fam"
while IFS="$(printf '\t')" read -r file line normowner owner pat; do
  [[ -n "$pat" ]] || continue
  stem=""
  if [[ "$pat" =~ $family_re ]]; then
    stem="${BASH_REMATCH[1]}"
  fi
  known=0
  if [[ -n "$stem" ]]; then
    for entry in "${canonical_patterns[@]}"; do
      if [[ "$stem" == "${entry%%|*}" ]]; then
        known=1
        break
      fi
    done
  fi
  if ((known == 1)); then
    printf '%s\t%d\t%s\t%s\t%s\n' "$file" "$line" "$normowner" "$owner" "$pat" >> "$fam"
  else
    printf '%s\t%d\t%s\t%s\t%s\n' "$file" "$line" "$normowner" "$owner" "$pat" >> "$nonfam"
  fi
done < "$rows"

dups="$workdir/dups.txt"
awk -F'\t' '{ print $3"\t"$5 }' "$nonfam" | sort -u | cut -f1 | sort | uniq -d > "$dups"
if [[ -s "$dups" ]]; then
  while IFS= read -r dup; do
    [[ -n "$dup" ]] || continue
    echo "SHARED-NAME-DIVERGENCE owner=$dup:" >&2
    awk -F'\t' -v o="$dup" '$3 == o { printf "  %s:%s owner=%s pattern=%s\n", $1, $2, $4, $5 }' "$nonfam" >&2
    fail=1
  done < "$dups"
fi

pattern_count="$(wc -l < "$rows" | tr -d '[:space:]')"
if ((fail != 0)); then
  echo >&2
  echo "OpenAPI scalar patterns diverged across specs" >&2
  echo "Pin the new pattern to its family canonical value in" >&2
  echo "scripts/openapi-pattern-check.sh, or fix the spec." >&2
  exit 1
fi

echo "openapi patterns: OK ($pattern_count pattern occurrences across ${#specs[@]} specs)"

# Rule 3: structural operation contract. Operation bodies always sit deeper
# than indent 4, so any indent-4 key ends the current operation, any
# indent-2 key ends the current path, and any top-level key ends path
# context. A mutation states its Idempotency-Key status when its block
# mentions idempotency (case-insensitive): the shared IdempotencyKey
# parameter, an Idempotency-Key description, or an explicit "No
# Idempotency-Key ..." / "... without ... idempotency ..." note.
allow_file="$workdir/idem_allow.txt"
: > "$allow_file"
if ((${#idem_status_allowlist[@]} > 0)); then
  printf '%s\n' "${idem_status_allowlist[@]}" > "$allow_file"
fi
rule3_out="$workdir/rule3.txt"
if ! awk -v allow_file="$allow_file" '
  BEGIN {
    while ((getline line < allow_file) > 0) {
      allow[line] = 1
    }
  }
  function flush_op(   key) {
    if (op_verb == "") {
      return
    }
    if (op_id == "") {
      printf "MISSING-OPERATIONID %s:%d %s %s\n", op_file, op_line, toupper(op_verb), op_path
      failed = 1
    }
    if (op_verb ~ /^(post|put|patch|delete)$/ && op_has_idem == 0) {
      key = toupper(op_verb) " " op_path " " op_id
      if (!(key in allow)) {
        printf "UNDOCUMENTED-IDEMPOTENCY %s:%d %s %s op=%s\n", op_file, op_line, toupper(op_verb), op_path, op_id
        failed = 1
      }
    }
    op_verb = ""
  }
  FNR == 1 {
    flush_op()
    op_path = ""
  }
  /^  \// {
    flush_op()
    line = $0
    sub(/^  /, "", line)
    sub(/:[[:space:]]*$/, "", line)
    op_path = line
    next
  }
  /^    [^ ]/ {
    flush_op()
    verb = $0
    sub(/^    /, "", verb)
    sub(/:[[:space:]]*$/, "", verb)
    if (verb ~ /^(get|post|put|patch|delete|head|options|trace)$/) {
      op_verb = verb
      op_file = FILENAME
      op_line = FNR
      op_id = ""
      op_has_idem = 0
    }
    next
  }
  /^ {0,4}[^ ]/ {
    flush_op()
    op_path = ""
    next
  }
  {
    if (op_verb != "") {
      if ($0 ~ /^[[:space:]]*operationId:[[:space:]]*[^[:space:]]/ && op_id == "") {
        idline = $0
        sub(/^[^:]*:[[:space:]]*/, "", idline)
        sub(/[[:space:]]+$/, "", idline)
        op_id = idline
      }
      if (tolower($0) ~ /idempotency/) {
        op_has_idem = 1
      }
    }
  }
  END {
    flush_op()
    exit failed
  }
' "${specs[@]}" > "$rule3_out"; then
  cat "$rule3_out" >&2
  echo >&2
  echo "OpenAPI operations violate the structural contract" >&2
  echo "Give the operation an operationId, or state its Idempotency-Key" >&2
  echo "requirement status (parameter or description)." >&2
  exit 1
fi

echo "openapi operations: OK (operationId + mutation idempotency status documented)"
