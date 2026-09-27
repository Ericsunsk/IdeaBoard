#!/usr/bin/env bash
# sql-workspace-scope-check-test.sh
#
# Self-test for scripts/sql-workspace-scope-check.sh. Builds synthetic query
# trees under a temp REPO_ROOT and asserts the exit code matches expectations.
# This is what the ops contract lane should invoke; without it, a bug in the
# scope checker would silently pass.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/sql-workspace-scope-check.sh"

new_tree() {
  local tmp
  tmp="$(mktemp -d)"
  mkdir -p "$tmp/apps/api/internal/db/queries"
  printf '%s' "$tmp"
}

# Case 1: the real tree passes.
if REPO_ROOT="$script_dir/.." bash "$checker" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

# Case 2: a new query without the scope predicate must fail.
tmp="$(new_tree)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/apps/api/internal/db/queries/workspace_tasks.sql" <<'EOF'
-- name: GetWorkspaceTaskByPublicID :one
SELECT t.internal_id FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- name: GetTaskWithoutScope :one
SELECT t.internal_id FROM tasks AS t
WHERE t.public_id = sqlc.arg(public_id)
LIMIT 1;
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: unscoped query should fail" >&2
  exit 1
fi
echo "OK: unscoped query fails"

# Case 3: projecting w.public_id without the WHERE predicate must fail.
# (This is the GetControlTaskByPublicID trap: a naive mentions-check passes it.)
cat > "$tmp/apps/api/internal/db/queries/workspace_tasks.sql" <<'EOF'
-- name: ListControlTasksClone :many
SELECT t.internal_id, w.public_id AS workspace_public_id
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
LIMIT sqlc.arg(row_limit);
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: projection-without-predicate should fail" >&2
  exit 1
fi
echo "OK: projection-without-predicate fails"

# Case 4: an allowlisted control-plane query passes.
cat > "$tmp/apps/api/internal/db/queries/workspace_tasks.sql" <<'EOF'
-- name: CountControlTasks :many
SELECT t.status AS status, COUNT(*) AS count
FROM tasks AS t
GROUP BY t.status;
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: allowlisted query passes"
else
  echo "FAIL: allowlisted query should pass" >&2
  exit 1
fi

# Case 5: a library query without workspace_id must fail.
cat > "$tmp/apps/api/internal/db/queries/workspace_library.sql" <<'EOF'
-- name: GetLibraryFolderByPublicID :one
SELECT internal_id FROM library_folders AS f
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND f.public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetFolderWithoutScope :one
SELECT internal_id FROM library_folders AS f
WHERE f.public_id = sqlc.arg(public_id)
LIMIT 1;
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: unscoped library query should fail" >&2
  exit 1
fi
echo "OK: unscoped library query fails"
