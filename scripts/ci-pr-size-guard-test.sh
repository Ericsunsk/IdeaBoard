#!/usr/bin/env bash
# ci-pr-size-guard-test.sh
#
# Contract test for scripts/ci-pr-size-guard.sh. Feeds synthetic
# changed-file lists through stdin (and the file-argument mode) and
# asserts the pass/warn/fail/exempt verdicts plus exit codes:
#   - small non-docs change passes,
#   - >50 files warns but exits 0,
#   - >150 files fails (exit 1),
#   - docs-only changes (README.md, docs/*, *.md) are exempt at any size,
#   - thresholds are strict greater-than (50 passes, 150 warns).
#
# Hermetic: bash, sort, grep, and seq only — no network or repo state.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
guard="${script_dir}/ci-pr-size-guard.sh"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "${fixture_dir}"' EXIT

code_list() {
  local count="$1"
  local i
  for ((i = 1; i <= count; i++)); do
    printf 'apps/api/internal/pkg/file%d.go\n' "${i}"
  done
}

docs_list() {
  local count="$1"
  local i
  for ((i = 1; i <= count; i++)); do
    case $((i % 3)) in
      0) printf 'docs/runbooks/guide%d.md\n' "${i}" ;;
      1) printf 'notes%d.md\n' "${i}" ;;
      *) printf 'README-%d.md\n' "${i}" ;;
    esac
  done
}

run_guard_stdin() {
  GITHUB_STEP_SUMMARY="${fixture_dir}/summary.md" \
  GITHUB_OUTPUT="${fixture_dir}/outputs.txt" \
    "${guard}"
}

# Case 1: empty input passes with zero files.
: >"${fixture_dir}/summary.md"
: >"${fixture_dir}/outputs.txt"
output="$(printf '' | run_guard_stdin)"
grep -Fq 'pr-size-guard: OK (0 files' <<<"${output}"
grep -Fq 'verdict=pass' "${fixture_dir}/outputs.txt"
grep -Fq 'file_count=0' "${fixture_dir}/outputs.txt"

# Case 2: small non-docs change passes.
: >"${fixture_dir}/summary.md"
: >"${fixture_dir}/outputs.txt"
output="$(printf '%s\n' apps/web/src/main.ts apps/api/go.mod | run_guard_stdin)"
grep -Fq 'pr-size-guard: OK (2 files' <<<"${output}"
grep -Fq 'docs_only=false' "${fixture_dir}/outputs.txt"
grep -Fq '| Verdict | pass |' "${fixture_dir}/summary.md"

# Case 3: exactly 50 files still passes (thresholds are strict >).
output="$(code_list 50 | run_guard_stdin)"
grep -Fq 'pr-size-guard: OK (50 files' <<<"${output}"

# Case 4: 51 files warns but exits 0.
: >"${fixture_dir}/outputs.txt"
output="$(code_list 51 | run_guard_stdin)"
grep -Fq '::warning::' <<<"${output}"
grep -Fq 'verdict=warn' "${fixture_dir}/outputs.txt"
grep -Fq 'file_count=51' "${fixture_dir}/outputs.txt"

# Case 5: exactly 150 files warns (fail tier needs strictly more).
output="$(code_list 150 | run_guard_stdin)"
grep -Fq '::warning::' <<<"${output}"
if code_list 150 | run_guard_stdin >/dev/null 2>&1; then
  :
else
  echo 'FAIL: 150 files must warn, not fail' >&2
  exit 1
fi

# Case 6: 151 files fails with ::error:: and exit 1.
: >"${fixture_dir}/outputs.txt"
if output="$(code_list 151 | run_guard_stdin 2>&1)"; then
  echo 'FAIL: 151 files should fail the size guard' >&2
  exit 1
fi
grep -Fq '::error::' <<<"${output}"
grep -Fq 'verdict=fail' "${fixture_dir}/outputs.txt"

# Case 7: docs-only changes are exempt at any size (200 files).
: >"${fixture_dir}/outputs.txt"
output="$(docs_list 200 | run_guard_stdin)"
grep -Fq 'pr-size-guard: exempt (200 docs-only files' <<<"${output}"
grep -Fq 'verdict=exempt' "${fixture_dir}/outputs.txt"
grep -Fq 'docs_only=true' "${fixture_dir}/outputs.txt"

# Case 8: mixed docs + code over the fail threshold still fails.
if { code_list 151; docs_list 10; } | run_guard_stdin >/dev/null 2>&1; then
  echo 'FAIL: mixed docs+code over the fail threshold must fail' >&2
  exit 1
fi

# Case 9: duplicate paths are counted once.
output="$(printf '%s\n' apps/web/src/main.ts apps/web/src/main.ts apps/api/go.mod | run_guard_stdin)"
grep -Fq 'pr-size-guard: OK (2 files' <<<"${output}"

# Case 10: file-argument mode matches stdin mode.
code_list 51 >"${fixture_dir}/changed-paths.txt"
: >"${fixture_dir}/outputs.txt"
output="$(GITHUB_STEP_SUMMARY="${fixture_dir}/summary.md" GITHUB_OUTPUT="${fixture_dir}/outputs.txt" "${guard}" "${fixture_dir}/changed-paths.txt")"
grep -Fq '::warning::' <<<"${output}"
grep -Fq 'verdict=warn' "${fixture_dir}/outputs.txt"

# Case 11: threshold overrides are honored.
output="$(code_list 7 | PR_SIZE_WARN_FILES=5 PR_SIZE_FAIL_FILES=10 run_guard_stdin)"
grep -Fq '::warning::' <<<"${output}"
if code_list 11 | PR_SIZE_WARN_FILES=5 PR_SIZE_FAIL_FILES=10 run_guard_stdin >/dev/null 2>&1; then
  echo 'FAIL: overridden fail threshold should reject 11 files' >&2
  exit 1
fi

# Case 12: invalid threshold configuration is a usage error, not a pass.
if code_list 3 | PR_SIZE_WARN_FILES=150 PR_SIZE_FAIL_FILES=150 run_guard_stdin >/dev/null 2>&1; then
  echo 'FAIL: warn threshold equal to fail threshold must be rejected' >&2
  exit 1
fi
if code_list 3 | PR_SIZE_WARN_FILES=soon run_guard_stdin >/dev/null 2>&1; then
  echo 'FAIL: non-integer threshold must be rejected' >&2
  exit 1
fi

echo 'CI PR size guard contract tests passed'
