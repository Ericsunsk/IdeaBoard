#!/usr/bin/env bash
# realtime-fetch-guard-test.sh
#
# The Fetch spec's bad-port list blocks port 6000, so global fetch can never
# reach the Go API. All Go internal calls must go through
# src/infrastructure/internalHttp.ts (internalFetch). This gate fails
# on any other fetch( call site in apps/realtime/src (tests excluded).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT_DIR/apps/realtime/src"

fail=0
while IFS= read -r hit; do
  file="${hit%%:*}"
  case "$file" in
    */infrastructure/internalHttp.ts | *.test.ts) continue ;;
  esac
  echo "realtime-fetch-guard: bare fetch( call in $hit (use internalFetch)" >&2
  fail=1
done < <(grep -rn --include="*.ts" -E "(^|[^a-zA-Z_.])fetch\(" "$SRC" | grep -v "\.test\.ts" || true)

# Self-test: the guard must be quiet on the current tree but loud on a
# planted violation.
probe_dir="$(mktemp -d)"
trap 'rm -rf "$probe_dir"' EXIT
mkdir -p "$probe_dir/src"
printf 'await fetch("http://x");\n' > "$probe_dir/src/bad.ts"
if grep -rn --include="*.ts" -E "(^|[^a-zA-Z_.])fetch\(" "$probe_dir/src" | grep -qv "\.test\.ts"; then
  : # correctly loud
else
  echo "realtime-fetch-guard: self-test failed (planted fetch( not detected)" >&2
  fail=1
fi

if ((fail != 0)); then
  echo "realtime-fetch-guard: FAILED" >&2
  exit 1
fi
echo "realtime-fetch-guard: OK"
