#!/usr/bin/env bash
# Guard v1 pagination, audit writes, settlement fakes, and canonical pool errors.
# Production files are checked except FAKE-TX, which also covers test files.
set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
api_root="$repo_root/apps/api"

if [ ! -d "$api_root/internal" ]; then
  echo "backend convergence check: $api_root/internal not found" >&2
  exit 1
fi

fail=0
report() {
  printf '  %s\n' "$1" >&2
}

# --- V1-OFFSET: forbid OFFSET / bare-cursor dialects in v1 production files.
v1_dir="$api_root/internal/runtime/interfaces/http/v1"
if [ -d "$v1_dir" ]; then
  while IFS= read -r file; do
    case "$file" in
      *_test.go) continue ;;
    esac
    # Legacy SQL offset.
    if grep -q 'OFFSET' "$file"; then
      report "V1-OFFSET $file: contains OFFSET (use opaque cursor + page_size via v1OffsetPage)"
      fail=1
    fi
    # Legacy limit/before_id helpers must never enter v1.
    for token in BeforeIDFromGin ParseBeforeID LimitFromGin ParseLimit BeforeIDParam LimitParam; do
      if grep -q "$token" "$file"; then
        report "V1-OFFSET $file: contains $token (v1 exposes only cursor + page_size)"
        fail=1
      fi
    done
    # Bare cursor literal bypasses the contract.CursorParam constant.
    if grep -Fq 'Query("cursor"' "$file"; then
      report "V1-OFFSET $file: contains bare literal Query(\"cursor\" (use contract.CursorParam + DecodeCursor)"
      fail=1
    fi
  done < <(find "$v1_dir" -name '*.go' | sort)
fi

# Check settlement fakes in every Go file and production-only rules afterward.
while IFS= read -r file; do
  if grep -Eq 'type[[:space:]]+[A-Za-z0-9_]*[Ff]ake[Tt]x[[:space:]]+struct' "$file"; then
    report "FAKE-TX $file: new FakeTx struct (reuse db/dbutil/testscope.Tx via alias)"
    fail=1
  fi
  case "$file" in
    *_test.go) continue ;;
  esac

  case "$file" in
    "$api_root/internal/platform/audit/"*|"$api_root/internal/db/sqlc/"*) ;;
    *)
      if grep -Eq 'INSERT INTO control_audit_records|UPDATE control_audit_records|DELETE FROM control_audit_records' "$file"; then
        report "AUDIT-WRITE $file: direct control_audit_records write (go through platform/audit)"
        fail=1
      fi
      ;;
  esac

  case "$file" in
    "$api_root/internal/platform/database/errors.go"|"$api_root/internal/billingrecords/ports/types.go") ;;
    *)
      if grep -Fq 'database pool not configured' "$file"; then
        report "POOL-LITERAL $file: new \"database pool not configured\" literal (use ErrPoolNotConfigured sentinel)"
        fail=1
      fi
      ;;
  esac
done < <(find "$api_root/internal" -name '*.go' | sort)

if [ "$fail" -ne 0 ]; then
  echo >&2
  echo "backend convergence guard violated" >&2
  echo "Fix the V1-OFFSET / AUDIT-WRITE / FAKE-TX / POOL-LITERAL lines above." >&2
  exit 1
fi

echo "backend convergence: OK"
