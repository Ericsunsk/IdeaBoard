#!/usr/bin/env bash
# backend-convergence-check-test.sh
#
# Self-test for scripts/backend-convergence-check.sh. Builds synthetic
# api trees under a temp REPO_ROOT and asserts the exit code matches
# expectations. Picked up by scripts/run-all-self-tests.sh glob; without
# it, a bug in the convergence guard would silently pass.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/backend-convergence-check.sh"

new_tree() {
  local tmp
  tmp="$(mktemp -d)"
  mkdir -p "$tmp/apps/api/internal/runtime/interfaces/http/v1"
  mkdir -p "$tmp/apps/api/internal/platform/audit"
  mkdir -p "$tmp/apps/api/internal/platform/database"
  mkdir -p "$tmp/apps/api/internal/billingrecords/ports"
  mkdir -p "$tmp/apps/api/internal/db/sqlc"
  printf '%s' "$tmp"
}

# Case 1: the real tree passes.
if REPO_ROOT="$script_dir/.." bash "$checker" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

# Case 2: OFFSET in v1 must fail.
tmp="$(new_tree)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1

func list() string {
	return "SELECT * FROM t LIMIT 10 OFFSET 5"
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: OFFSET in v1 should fail" >&2
  exit 1
fi
echo "OK: OFFSET in v1 fails"

# Case 3: bare Query("cursor" literal in v1 must fail.
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1

func list(c *gin.Context) string {
	return c.Query("cursor")
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: bare cursor literal in v1 should fail" >&2
  exit 1
fi
echo "OK: bare cursor literal in v1 fails"

# Case 4: legacy before_id helper in v1 must fail.
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1

func list(c *gin.Context) {
	_, _ = contract.BeforeIDFromGin(c)
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: BeforeIDFromGin in v1 should fail" >&2
  exit 1
fi
echo "OK: legacy before_id helper in v1 fails"

# Case 5: direct control_audit write outside platform/audit must fail.
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1
EOF
mkdir -p "$tmp/apps/api/internal/tasking"
cat > "$tmp/apps/api/internal/tasking/store.go" <<'EOF'
package tasking

const q = `INSERT INTO control_audit_records (public_id) VALUES ($1)`
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: direct control_audit write should fail" >&2
  exit 1
fi
echo "OK: direct control_audit write fails"
rm -rf "$tmp/apps/api/internal/tasking"

# Case 6: new FakeTx struct must fail; alias to testscope.Tx passes.
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1

type v1FakeTx struct {
	committed bool
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: FakeTx struct should fail" >&2
  exit 1
fi
echo "OK: FakeTx struct fails"
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1

type v1FakeTx = testscope.Tx
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: FakeTx alias passes"
else
  echo "FAIL: FakeTx alias should pass" >&2
  exit 1
fi

# Case 7: new pool literal must fail; canonical sentinels pass.
cat > "$tmp/apps/api/internal/runtime/interfaces/http/v1/list.go" <<'EOF'
package v1
EOF
mkdir -p "$tmp/apps/api/internal/credits"
cat > "$tmp/apps/api/internal/credits/service.go" <<'EOF'
package credits

var err = "database pool not configured"
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: pool literal should fail" >&2
  exit 1
fi
echo "OK: pool literal fails"
rm -rf "$tmp/apps/api/internal/credits"
cat > "$tmp/apps/api/internal/platform/database/errors.go" <<'EOF'
package database

const msg = "database pool not configured"
EOF
cat > "$tmp/apps/api/internal/billingrecords/ports/types.go" <<'EOF'
package ports

const msg = "database pool not configured"
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: canonical pool sentinels pass"
else
  echo "FAIL: canonical pool sentinels should pass" >&2
  exit 1
fi
