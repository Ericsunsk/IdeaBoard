#!/usr/bin/env bash
# web-frontend-env-check-test.sh
#
# Self-test for scripts/web-frontend-env-check.sh. Builds minimal fixture
# repo roots (env.d.ts + .env.example + src + functions) under a temp dir
# and asserts the checker accepts a clean tree while rejecting an undeclared
# read, a stale declaration, an undocumented key, a secret-looking name, a
# PUBLIC_* read, a committed example value, and an unexpected Functions
# binding. Hermetic: only bash/grep/sort, no network, secrets, or Docker.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

checker="$SCRIPT_DIR/web-frontend-env-check.sh"

fixture_root="$(mktemp -d)"
cleanup_path "$fixture_root"

make_green() {
  local root="$1"
  mkdir -p "$root/apps/web/src" "$root/apps/web/functions"
  cat >"$root/apps/web/src/env.d.ts" <<'EOF'
declare global {
  interface ImportMetaEnv {
    readonly VITE_API_BASE_URL?: string
    readonly VITE_CANVAS_PERF_PROBE?: string
  }
}
export {}
EOF
  cat >"$root/apps/web/.env.example" <<'EOF'
VITE_API_BASE_URL=
# VITE_CANVAS_PERF_PROBE=true
EOF
  cat >"$root/apps/web/src/urls.ts" <<'EOF'
const base = import.meta.env.VITE_API_BASE_URL
const probe = import.meta.env.VITE_CANVAS_PERF_PROBE
export { base, probe }
EOF
  cat >"$root/apps/web/functions/_proxy.ts" <<'EOF'
const origin = env.IDEABOARD_API_ORIGIN
export { origin }
EOF
}

expect_pass() {
  if ! bash "$checker" "$1" >/dev/null 2>&1; then
    echo "web-frontend-env-check-test: expected PASS for $2" >&2
    return 1
  fi
}

expect_fail_with() {
  # NOTE: capture to a variable first — `checker | grep -q` under
  # `set -o pipefail` is racy (grep -q closes the pipe early, checker dies
  # of SIGPIPE with 141 and the pipeline status becomes schedule-dependent).
  local out rc
  set +e
  out="$(bash "$checker" "$1" 2>&1)"
  rc=$?
  set -e
  if ((rc == 0)); then
    echo "web-frontend-env-check-test: expected FAIL for $2" >&2
    return 1
  fi
  if ! grep -q "$3" <<<"$out"; then
    echo "web-frontend-env-check-test: missing marker '$3' for $2" >&2
    return 1
  fi
}

green="$fixture_root/green"
make_green "$green"
expect_pass "$green" "clean tree"

undeclared="$fixture_root/undeclared"
make_green "$undeclared"
printf '%s\n' "const x = import.meta.env.VITE_NEW_THING" >>"$undeclared/apps/web/src/urls.ts"
expect_fail_with "$undeclared" "undeclared read" "not declared in apps/web/src/env.d.ts"

stale="$fixture_root/stale"
make_green "$stale"
printf '%s\n' '    readonly VITE_STALE_KEY?: string' >>"$stale/apps/web/src/env.d.ts"
printf '%s\n' 'VITE_STALE_KEY=' >>"$stale/apps/web/.env.example"
expect_fail_with "$stale" "stale declaration" "never read in apps/web/src"

undoc="$fixture_root/undoc"
make_green "$undoc"
printf '%s\n' '    readonly VITE_UNDOC?: string' >>"$undoc/apps/web/src/env.d.ts"
printf '%s\n' "const u = import.meta.env.VITE_UNDOC" >>"$undoc/apps/web/src/urls.ts"
expect_fail_with "$undoc" "undocumented key" "not documented in apps/web/.env.example"

secretn="$fixture_root/secretname"
make_green "$secretn"
printf '%s\n' '    readonly VITE_API_TOKEN?: string' >>"$secretn/apps/web/src/env.d.ts"
printf '%s\n' "const t = import.meta.env.VITE_API_TOKEN" >>"$secretn/apps/web/src/urls.ts"
printf '%s\n' 'VITE_API_TOKEN=' >>"$secretn/apps/web/.env.example"
expect_fail_with "$secretn" "secret-looking name" "looks like a secret"

pubread="$fixture_root/pubread"
make_green "$pubread"
printf '%s\n' "const p = import.meta.env.PUBLIC_ORIGIN" >>"$pubread/apps/web/src/urls.ts"
expect_fail_with "$pubread" "PUBLIC_* read" "only exposes VITE_"

committed="$fixture_root/committed"
make_green "$committed"
printf '%s\n' 'VITE_COMMITTED_VALUE=https://api.example.com' >>"$committed/apps/web/.env.example"
printf '%s\n' '    readonly VITE_COMMITTED_VALUE?: string' >>"$committed/apps/web/src/env.d.ts"
printf '%s\n' "const c = import.meta.env.VITE_COMMITTED_VALUE" >>"$committed/apps/web/src/urls.ts"
expect_fail_with "$committed" "committed example value" "never from git"

binding="$fixture_root/binding"
make_green "$binding"
printf '%s\n' 'const s = env.IDEABOARD_EXTRA_SECRET' >>"$binding/apps/web/functions/_proxy.ts"
expect_fail_with "$binding" "unexpected Functions binding" "unexpected binding IDEABOARD_EXTRA_SECRET"

echo "web-frontend-env-check self-test passes"
