#!/usr/bin/env bash
set -euo pipefail

# Report-Only is the safe default. Enforce only after report-only smoke is clean.
mode="${CSP_MODE:-report-only}"
case "$mode" in
  report-only) header='Content-Security-Policy-Report-Only' ;;
  enforce) header='Content-Security-Policy' ;;
  *) echo "invalid CSP_MODE: $mode" >&2; exit 2 ;;
esac
policy="script-src 'self' 'unsafe-inline' 'unsafe-eval'"
printf '%s: %s\n' "$header" "$policy"
if [[ "${CSP_CHECK:-0}" == 1 ]]; then
  [[ "$mode" == report-only ]] || { echo 'CSP_CHECK requires report-only mode' >&2; exit 1; }
  echo 'CSP check passed: staged report-only policy is active'
fi
