#!/usr/bin/env bash
# sql-workspace-scope-check.sh
#
# Workspace SQL scope guard (Option B: no migration, no semantics change).
#
# Every query in apps/api/internal/db/queries/workspace_{tasks,media,boards}.sql
# repeats the workspace-scoping join by hand, so one missed predicate is a
# cross-workspace leak. sqlc has no include/fragment mechanism, and a
# v_workspace_* view cannot cover the mutations (UPDATE...FROM / DELETE USING
# still need the predicate in WHERE, and views would change sqlc-generated Go
# types), so the smallest change that kills the leak class is this grep guard:
# each query block must carry the scope predicate, or be explicitly allowlisted
# below with its safety reason.
#
# Rules:
#   workspace_{tasks,media,boards}.sql: block must contain the predicate
#     w.public_id = sqlc.arg(workspace_public_id)
#   workspace_library.sql: block must contain sqlc.arg(workspace_id)
#     (library stores resolve the workspace internal id once at the entry
#     seam and key every lookup by (workspace_id, public_id)).
#
# The predicate match is whitespace-normalized, but it must be the predicate:
# projecting w.public_id (e.g. AS workspace_public_id in control-plane reads)
# without the WHERE predicate does NOT pass.
#
# This script is intentionally unwired from CI (CI workflows are owned
# separately). To wire it, mirror the sibling contract lanes: invoke
# scripts/sql-workspace-scope-check-test.sh from .github/workflows/ops.yml
# ("Verify CI orchestration contracts"), and add both scripts to the
# bash -n / shellcheck lists in that file.
#
set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
query_dir="$repo_root/apps/api/internal/db/queries"

# Queries that legitimately carry no workspace scope predicate, with reasons.
# Keep this list minimal: adding a name here weakens the guard for that query.
predicate_allowlist=(
  # Control-plane global read model: no membership gating by design; the
  # control guard owns capability auth and every call is audited.
  "GetControlTaskByPublicID"
  "ListControlTasks"
  "CountControlTasks"
  "GetControlTaskBilling"
  # Final-timeout settlement sweep DISCOVERY: cross-workspace by design — an
  # abandoned Credit Hold locks a specific user's credits, so scoping the scan
  # to one workspace would leave every other workspace's users locked
  # forever. It is read-only and INNER JOINs workspaces (only rows whose
  # workspace resolves are visible); every row it returns is converged through
  # the workspace-scoped FailWorkspaceTask, so a mismatched pair fails closed
  # instead of settling cross-workspace.
  "ListStalledHeldWorkspaceTasks"
  # Inserts taking the caller-resolved workspace internal id: the store
  # resolves the workspace once at the entry seam, so there is no public_id
  # join left to scope.
  "CreateWorkspaceTask"
  "CreateUploadSession"
  "CreateMediaAsset"
  # Internal-id keyed: ids are resolved from an in-scope read in the same
  # store call, never from caller-supplied public ids.
  "ListWorkspaceTaskOutputsByTaskIDs"
  "TouchWorkspaceBoard"
  "InsertBoardDocument"
  "UpdateBoardDocument"
  "InsertBoardRevision"
  # Board preview backfill sweep (operator command only, no user input):
  # the scan pages internal board ids with no assigned content identity
  # and the write re-keys by the scanned internal id with an IS NULL
  # guard, so concurrent runs converge without touching other rows.
  "ScanBoardDocumentsMissingPreview"
  "UpdateBoardDocumentPreview"
)

fail=0

# check_file <file> <pattern>: every -- name: block must contain pattern
# (after whitespace normalization) unless the query is allowlisted.
check_file() {
  local file="$1"
  local pattern="$2"
  local allow="$3"
  if [ ! -f "$file" ]; then
    return 0
  fi
  local out
  if ! out="$(awk -v pat="$pattern" -v allowlist=" $allow " '
    /^-- name:[ \t]+[A-Za-z0-9_]+/ {
      if (q != "") check(q, buf);
      q = $3; buf = "";
      next;
    }
    { buf = buf "\n" $0 }
    END { if (q != "") check(q, buf) }
    function check(name, body,   n) {
      if (index(allowlist, " " name " ") > 0) return;
      n = body;
      gsub(/[ \t]+/, " ", n);
      if (index(n, pat) == 0) {
        printf "MISSING-SCOPE %s\n", name;
        failed = 1;
      }
    }
    END { if (failed) exit 1 }
  ' "$file")"; then
    echo "✗ $file:" >&2
    while IFS= read -r line; do printf '  %s\n' "$line" >&2; done <<< "$out"
    fail=1
    return 0
  fi
  echo "OK: $file"
}

allow_str="${predicate_allowlist[*]}"
check_file "$query_dir/workspace_tasks.sql" 'w.public_id = sqlc.arg(workspace_public_id)' "$allow_str"
check_file "$query_dir/workspace_media.sql" 'w.public_id = sqlc.arg(workspace_public_id)' "$allow_str"
check_file "$query_dir/workspace_boards.sql" 'w.public_id = sqlc.arg(workspace_public_id)' "$allow_str"
# Library keys every lookup by the resolved workspace internal id instead of
# the public_id join; no allowlist entries exist today (fail closed on new ones).
check_file "$query_dir/workspace_library.sql" 'sqlc.arg(workspace_id)' ""

if [ "$fail" -ne 0 ]; then
  echo >&2
  echo "workspace SQL scope violated" >&2
  echo "Every workspace_*.sql query must carry its scope predicate, or be added" >&2
  echo "to the explicit allowlist in scripts/sql-workspace-scope-check.sh with a reason." >&2
  exit 1
fi

echo "workspace SQL scope: OK"
