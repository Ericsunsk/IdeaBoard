#!/usr/bin/env bash
# ci-media-package-boundary.sh
#
# Issue #667 / canonical media object contract: the Workflow and
# Worker packages must consume the media subsystem only through the
# typed ports. They MAY NOT import the concrete media/storage
# package — that would let a workflow feature reach into a specific
# storage adapter, breaking the "Runtime → contexts → ports →
# adapters" dependency direction and the package boundary the runbook
# pins as a cutover gate.
#
# This script is wired into .github/workflows/api.yml as a contract
# lane. It walks every non-test Go file under the forbidden packages
# and fails if it imports the concrete media/storage path.
#
set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"

# Concrete storage import that is forbidden in non-test code under
# these packages.
forbidden_path='ideaboard/api/internal/media/storage'

# Packages that may NOT import the concrete storage adapter outside
# the whitelist. Each entry is the path under apps/api/...
forbidden_packages=(
  "internal/runtime/worker"
  "internal/workflow"
)

fail=0
for pkg in "${forbidden_packages[@]}"; do
  full_pkg_path="$repo_root/apps/api/$pkg"
  if [ ! -d "$full_pkg_path" ]; then
    continue
  fi
  # Tests are composition roots and are intentionally outside this
  # production package-dependency rule.
  while IFS= read -r -d '' file; do
    if [[ "$file" == *_test.go ]]; then
      continue
    fi
    rel="${file#"$repo_root"/apps/api/}"
    if grep -q "\"$forbidden_path" "$file"; then
      echo "✗ forbidden import of $forbidden_path in $rel" >&2
      fail=1
    fi
  done < <(find "$full_pkg_path" -maxdepth 2 -name '*.go' -not -name '*_test.go' -print0)
done

if [ "$fail" -ne 0 ]; then
  echo >&2
  echo "media package boundary violated" >&2
  echo "Workflow/Worker must consume media through typed ports only." >&2
  echo "Add or extend a typed port; do not introduce a compatibility allowlist." >&2
  exit 1
fi

echo "media package boundary: OK"
