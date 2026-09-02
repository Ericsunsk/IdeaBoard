#!/usr/bin/env bash
# ci-media-package-boundary-test.sh
#
# Self-test for scripts/ci-media-package-boundary.sh. The script
# intentionally uses a synthetic forbidden-package tree, runs the
# check, and asserts the exit code matches expectations. This is
# what the ci-contract lane invokes; without it, a bug in the
# boundary checker would silently pass.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
boundary="$script_dir/ci-media-package-boundary.sh"

# Case 1: a clean tree passes.
if REPO_ROOT="$script_dir/.." bash "$boundary" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

# Case 2: a synthetic file that imports media/storage in a
# forbidden package must fail. Build a temp tree that mimics the
# forbidden path and let the boundary script run over it.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/apps/api/internal/runtime/worker"
cat > "$tmp/apps/api/internal/runtime/worker/bad.go" <<'EOF'
package worker

import (
  _ "ideaboard/api/internal/media/storage"
)

var _ = struct{}{}
EOF

if REPO_ROOT="$tmp" bash "$boundary" >/dev/null 2>&1; then
  echo "FAIL: bad tree should fail" >&2
  exit 1
fi
echo "OK: bad tree fails"
