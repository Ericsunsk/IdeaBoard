#!/usr/bin/env bash
# ci-pr-size-guard.sh
#
# PR size gate: keep changes reviewable.
#
# Reads a newline-separated changed-file list and enforces:
#   - file count > PR_SIZE_WARN_FILES (default 50):  ::warning:: annotation, exit 0.
#   - file count > PR_SIZE_FAIL_FILES (default 150): ::error:: annotation, exit 1.
#   - docs-only changes (README.md, docs/*, *.md — the same rule as
#     scripts/ci-change-classifier.sh) are exempt at any size, exit 0.
#
# Input mirrors scripts/ci-change-classifier.sh: newline-separated paths
# on stdin by default, or a file path argument (e.g. the
# changed-paths.txt the CI `changes` lane materializes). Duplicate paths
# are counted once.
#
# This script is wired into .github/workflows/ci.yml as the always-run
# `PR Size Guard` lane; failures block via the CI Gate lane. It honors
# GITHUB_STEP_SUMMARY and GITHUB_OUTPUT when set.
#
# Usage:
#   git diff --name-only "${BASE_SHA}" "${HEAD_SHA}" | ./scripts/ci-pr-size-guard.sh
#   ./scripts/ci-pr-size-guard.sh "${RUNNER_TEMP}/changed-paths.txt"
#
set -euo pipefail

warn_threshold="${PR_SIZE_WARN_FILES:-50}"
fail_threshold="${PR_SIZE_FAIL_FILES:-150}"

[[ "${warn_threshold}" =~ ^[0-9]+$ ]] || {
  echo "ci-pr-size-guard: PR_SIZE_WARN_FILES must be a non-negative integer, got ${warn_threshold@Q}" >&2
  exit 2
}
[[ "${fail_threshold}" =~ ^[0-9]+$ ]] || {
  echo "ci-pr-size-guard: PR_SIZE_FAIL_FILES must be a non-negative integer, got ${fail_threshold@Q}" >&2
  exit 2
}
((warn_threshold < fail_threshold)) || {
  echo "ci-pr-size-guard: PR_SIZE_WARN_FILES (${warn_threshold}) must be less than PR_SIZE_FAIL_FILES (${fail_threshold})" >&2
  exit 2
}

input_source='stdin'
if (($# >= 1)); then
  input_source="$1"
  [[ -f "${input_source}" ]] || {
    echo "ci-pr-size-guard: changed-file list not found: ${input_source}" >&2
    exit 2
  }
fi

file_count=0
non_docs_count=0
if (($# >= 1)); then
  sorted_input="$(sort -u -- "${input_source}")"
else
  sorted_input="$(sort -u)"
fi
while IFS= read -r changed_path; do
  [[ -n "${changed_path}" ]] || continue
  file_count=$((file_count + 1))
  case "${changed_path}" in
    README.md | docs/* | *.md) ;;
    *) non_docs_count=$((non_docs_count + 1)) ;;
  esac
done <<<"${sorted_input}"

if ((file_count == 0)); then
  docs_only=true
else
  docs_only=false
  ((non_docs_count == 0)) && docs_only=true
fi

verdict=pass
if ((file_count == 0)); then
  verdict=pass
elif [[ "${docs_only}" == true ]]; then
  verdict=exempt
elif ((file_count > fail_threshold)); then
  verdict=fail
elif ((file_count > warn_threshold)); then
  verdict=warn
fi

summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
{
  echo '## PR size guard'
  echo
  echo "| Metric | Value |"
  echo '| --- | --- |'
  echo "| Changed files | ${file_count} |"
  echo "| Docs only | ${docs_only} |"
  echo "| Warn threshold | >${warn_threshold} |"
  echo "| Fail threshold | >${fail_threshold} |"
  echo "| Verdict | ${verdict} |"
} >>"${summary_file}"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    printf 'file_count=%s\n' "${file_count}"
    printf 'docs_only=%s\n' "${docs_only}"
    printf 'verdict=%s\n' "${verdict}"
  } >>"${GITHUB_OUTPUT}"
fi

case "${verdict}" in
  pass)
    echo "pr-size-guard: OK (${file_count} files, docs_only=${docs_only})"
    ;;
  exempt)
    echo "pr-size-guard: exempt (${file_count} docs-only files; no non-docs changes)"
    ;;
  warn)
    echo "::warning::pr-size-guard: ${file_count} changed files exceed ${warn_threshold}; consider splitting this PR for reviewability."
    echo "pr-size-guard: warning (${file_count} files exceed warn threshold ${warn_threshold})"
    ;;
  fail)
    echo "::error::pr-size-guard: ${file_count} changed files exceed ${fail_threshold}; split this PR before review."
    echo "pr-size-guard: FAILED (${file_count} files exceed fail threshold ${fail_threshold})" >&2
    exit 1
    ;;
esac
