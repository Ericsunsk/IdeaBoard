#!/usr/bin/env bash
# openapi-breaking-check-test.sh
#
# Self-test for scripts/openapi-breaking-check.sh covering only the hermetic
# pre-gate paths (approval-label skip, missing base-ref skip, missing head
# spec failure). Every case exits before the oasdiff install, so this test
# needs no network and no Go toolchain. Full diff behavior stays covered by
# the api.yml CI lane with real specs.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

fixture_dir="$(mktemp -d)"
cleanup_path "$fixture_dir"

checker="$SCRIPT_DIR/openapi-breaking-check.sh"

# Approval label short-circuits before any spec or network access.
output="$(OPENAPI_BREAKING_LABELS="openapi-breaking-approved" bash "$checker" "HEAD" "$fixture_dir/absent.yaml" 2>&1)"
[[ "$output" == *"skipped (openapi-breaking-approved label present)"* ]] || {
  echo "FAIL: approval label did not skip the gate: $output" >&2
  exit 1
}

# Empty base ref (workflow_dispatch without history) skips without network.
output="$(bash "$checker" "" 2>&1)"
[[ "$output" == *"skipped (no base ref"* ]] || {
  echo "FAIL: empty base ref did not skip the gate: $output" >&2
  exit 1
}

# All-zero base ref (shallow/empty history) skips the same way.
output="$(bash "$checker" "0000000000000000000000000000000000000000" 2>&1)"
[[ "$output" == *"skipped (no base ref"* ]] || {
  echo "FAIL: zero base ref did not skip the gate: $output" >&2
  exit 1
}

# A missing head spec is a hard failure, never a silent skip.
if output="$(bash "$checker" "HEAD" "$fixture_dir/absent.yaml" 2>&1)"; then
  echo "FAIL: missing head spec passed the gate" >&2
  exit 1
fi
[[ "$output" == *"is missing"* ]] || {
  echo "FAIL: missing head spec reported the wrong error: $output" >&2
  exit 1
}

echo "openapi-breaking-check contract tests passed"
