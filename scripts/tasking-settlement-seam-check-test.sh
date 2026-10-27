#!/usr/bin/env bash
# tasking-settlement-seam-check-test.sh
#
# Self-test for scripts/tasking-settlement-seam-check.sh. Builds synthetic
# tasking trees under a temp REPO_ROOT and asserts the exit code matches
# expectations. This is what the ops contract lane runs (picked up by
# scripts/run-all-self-tests.sh glob); without it, a bug in the seam
# guard would silently pass.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/tasking-settlement-seam-check.sh"

new_tree() {
  local tmp
  tmp="$(mktemp -d)"
  mkdir -p "$tmp/apps/api/internal/tasking/workspacetasks"
  printf '%s' "$tmp"
}

# Case 1: the real tree passes.
if REPO_ROOT="$script_dir/.." bash "$checker" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

# Case 2: nil,false,nil inside an *InTx seam must fail.
tmp="$(new_tree)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/apps/api/internal/tasking/workspacetasks/seam.go" <<'EOF'
package workspacetasks

import "github.com/jackc/pgx/v5"

func (s *Store) ClaimSubmissionInTx(tx pgx.Tx) (*Claim, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	return nil, true, nil
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: nil,false,nil in InTx should fail" >&2
  exit 1
fi
echo "OK: nil,false,nil in InTx fails"

# Case 3: nil,nil inside an *InTx seam must fail.
cat > "$tmp/apps/api/internal/tasking/workspacetasks/seam.go" <<'EOF'
package workspacetasks

import "github.com/jackc/pgx/v5"

func (s *Service) CompleteTaskInTx(tx pgx.Tx) (*Task, error) {
	if s == nil {
		return nil, errNope
	}
	return nil, nil
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: nil,nil in InTx should fail" >&2
  exit 1
fi
echo "OK: nil,nil in InTx fails"

# Case 4: nil outbox guard returning bare nil in an *OutboxTx writer must fail.
cat > "$tmp/apps/api/internal/tasking/workspacetasks/seam.go" <<'EOF'
package workspacetasks

import "github.com/jackc/pgx/v5"

func (s *Store) insertTaskOutboxTx(tx pgx.Tx) error {
	if s == nil || s.outbox == nil {
		return nil
	}
	return s.outbox.InsertTx(tx)
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: nil outbox returning nil should fail" >&2
  exit 1
fi
echo "OK: nil outbox returning nil fails"

# Case 5: legitimate shapes pass — claim-success (nil,true,nil), the
# zero-leg marker ("",nil), non-InTx nil,false,nil (decode-style ok=false),
# and a fail-closed nil-guard returning a wrapped error.
cat > "$tmp/apps/api/internal/tasking/workspacetasks/seam.go" <<'EOF'
package workspacetasks

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ClaimSubmissionInTx(tx pgx.Tx) (*Claim, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("nope: %w", errNope)
	}
	return nil, true, nil
}

func (s *Store) insertTaskOutboxTx(tx pgx.Tx) error {
	if s == nil || s.outbox == nil {
		return fmt.Errorf("nope: %w", errNope)
	}
	return s.outbox.InsertTx(tx)
}

func decodeInline(raw []byte) (data []byte, ok bool, err error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	return raw, true, nil
}

func (h *Holder) CaptureInTx(tx pgx.Tx, amount int64) (string, error) {
	if amount <= 0 {
		return "", nil
	}
	return h.capture(tx)
}
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: legitimate shapes pass"
else
  echo "FAIL: legitimate shapes should pass" >&2
  exit 1
fi
