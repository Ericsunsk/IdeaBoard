#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
headers_file="${script_dir}/../apps/web/public/_headers"
grep -q '^/\*$' "$headers_file"
grep -Fq 'Content-Security-Policy-Report-Only:' "$headers_file"
if grep -q '^  Content-Security-Policy:' "$headers_file"; then
  echo '_headers enables enforced CSP before staged gate' >&2
  exit 1
fi
report="$(CSP_CHECK=1 "$script_dir/csp-policy.sh")"
grep -Fq 'Content-Security-Policy-Report-Only:' <<<"$report"
grep -Fq 'CSP check passed' <<<"$report"
enforced="$(CSP_MODE=enforce "$script_dir/csp-policy.sh")"
grep -Fq 'Content-Security-Policy:' <<<"$enforced"
if CSP_MODE=invalid "$script_dir/csp-policy.sh" >/dev/null 2>&1; then echo 'invalid CSP mode accepted' >&2; exit 1; fi
if CSP_MODE=enforce CSP_CHECK=1 "$script_dir/csp-policy.sh" >/dev/null 2>&1; then echo 'enforce mode passed staged check' >&2; exit 1; fi
echo 'CSP policy contract tests passed'
