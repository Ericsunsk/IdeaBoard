#!/usr/bin/env bash
# run-all-self-tests.sh
#
# Manifest runner for every scripts/*-test.sh self-test. CI (ops.yml) invokes
# this single entry point, so adding a new *-test.sh needs no workflow edit:
# it is picked up by the sorted glob below. The bash -n and ShellCheck
# checks in ops.yml share the tracked scripts/*.sh inventory.
#
# Ordering: sorted-glob order is safe because every self-test was verified to
# be side-effect free across tests — each runs in its own `bash` process (no
# exported state leaks), all writes are confined to mktemp fixtures, and the
# only `cd` is inside a subshell (ci-workflow-contract-test.sh). If a future
# test depends on another's side effects, pin its explicit order here with a
# comment instead of relying on the glob.
#
# Hermeticity: every *-test.sh as of this writing needs no network, secrets,
# or Docker daemon (docker/go/npm/atlas/curl are faked on PATH inside
# fixtures; only jq, git, and the Go toolchain are real, all present in the
# ops job). A future non-hermetic test must be skipped here with a reason
# comment — never silently.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

tests=( "$SCRIPT_DIR"/*-test.sh )
[[ -e "${tests[0]}" ]] || die "no *-test.sh self-tests found under $SCRIPT_DIR"

passed=0
failures=0
for test in "${tests[@]}"; do
  name="$(basename "$test")"
  if bash "$test"; then
    printf 'PASS: %s\n' "$name"
    passed=$((passed + 1))
  else
    printf 'FAIL: %s\n' "$name" >&2
    failures=$((failures + 1))
  fi
done

printf 'self-tests: %d passed, %d failed\n' "$passed" "$failures"
((failures == 0)) || die "${failures} self-test(s) failed"
