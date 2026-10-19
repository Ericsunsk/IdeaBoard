#!/usr/bin/env bash
# handbook-version-contract-test.sh
#
# Self-test for scripts/handbook-version-contract.sh (issue #855).
# Case 1 runs the check against the real working tree; the remaining
# cases use synthetic fixture trees and assert the exit code matches
# expectations. Without these, a bug in the contract checker would
# silently pass version drift or stale handbook entry points.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
contract="$script_dir/handbook-version-contract.sh"

# Case 1: the real tree passes (it carries the #855 repairs).
if REPO_ROOT="$script_dir/.." bash "$contract" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

# Minimal fixture tree that satisfies every contract clause.
make_fixture() {
  local root="$1"
  mkdir -p "$root/apps/auth" "$root/apps/realtime" "$root/apps/api" "$root/docs/runbooks"
  printf '22.23.2' >"$root/.node-version"
  printf 'module ideaboard/api\n\ngo 1.26.6\n' >"$root/apps/api/go.mod"
  printf '# syntax=docker/dockerfile:1.7\n\nARG NODE_VERSION=22.23.2\n' >"$root/apps/auth/Dockerfile"
  printf '# syntax=docker/dockerfile:1.7\n\nARG NODE_VERSION=22.23.2\n' >"$root/apps/realtime/Dockerfile"
  # shellcheck disable=SC2016 # $BUILDPLATFORM is a Dockerfile literal in the fixture.
  printf '# syntax=docker/dockerfile:1.7\n\nFROM --platform=$BUILDPLATFORM golang:1.26.6-bookworm AS builder\n' >"$root/apps/api/Dockerfile"
  # shellcheck disable=SC2016 # Backticks are markdown code spans in the fixture.
  printf '# Handbook\n\nSee root `.node-version` and `apps/api/go.mod`.\n\n[entry](./real-target.md)\n' >"$root/DEVELOPMENT.md"
  printf '# stub\n' >"$root/DESIGN.md"
  # shellcheck disable=SC2016 # Backticks are markdown code spans in the fixture.
  printf '# stub referencing `.node-version` and `apps/api/go.mod`\n' >"$root/README.md"
  printf '# stub\n' >"$root/AGENTS.md"
  # shellcheck disable=SC2016 # Backticks are markdown code spans in the fixture.
  printf '# stub referencing `.node-version`\n' >"$root/docs/runbooks/deployment.md"
  printf '# target\n' >"$root/real-target.md"
}

expect_fail() {
  local label="$1" root="$2"
  if REPO_ROOT="$root" bash "$contract" >/dev/null 2>&1; then
    echo "FAIL: ${label} should fail" >&2
    exit 1
  fi
  echo "OK: ${label} fails"
}

# Case 2: drifted auth Dockerfile Node pin must fail.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
make_fixture "$tmp/clean"
cp -r "$tmp/clean" "$tmp/node-drift"
printf '# syntax=docker/dockerfile:1.7\n\nARG NODE_VERSION=20.19.0\n' >"$tmp/node-drift/apps/auth/Dockerfile"
expect_fail "node pin drift" "$tmp/node-drift"

# Case 3: drifted api Dockerfile Go pin must fail.
cp -r "$tmp/clean" "$tmp/go-drift"
# shellcheck disable=SC2016 # $BUILDPLATFORM is a Dockerfile literal in the fixture.
printf '# syntax=docker/dockerfile:1.7\n\nFROM --platform=$BUILDPLATFORM golang:1.25.0-bookworm AS builder\n' >"$tmp/go-drift/apps/api/Dockerfile"
expect_fail "go pin drift" "$tmp/go-drift"

# Case 4: broken handbook entry-point link must fail.
cp -r "$tmp/clean" "$tmp/bad-link"
# shellcheck disable=SC2016 # Backticks are markdown code spans in the fixture.
printf '# Handbook\n\nSee root `.node-version` and `apps/api/go.mod`.\n\n[gone](./does-not-exist.md)\n' >"$tmp/bad-link/DEVELOPMENT.md"
expect_fail "broken handbook link" "$tmp/bad-link"

# Case 5: duplicated version literal in handbook docs must fail.
cp -r "$tmp/clean" "$tmp/bad-literal"
# shellcheck disable=SC2016 # Backticks are markdown code spans in the fixture.
printf '# stub with Node.js `22.23.2` literal and `apps/api/go.mod`\n' >"$tmp/bad-literal/README.md"
expect_fail "duplicated version literal" "$tmp/bad-literal"

echo "handbook version contract tests passed"
