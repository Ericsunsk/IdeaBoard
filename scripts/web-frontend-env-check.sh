#!/usr/bin/env bash
# web-frontend-env-check.sh
#
# Frontend env drift gate: Vite bakes every `VITE_*` value into the shipped
# bundle at build time, so a `VITE_*` key is public by construction and must
# never carry a secret. The code-owned schema is `apps/web/src/env.d.ts`
# (the `readonly VITE_*` declarations); usage is every
# `import.meta.env.VITE_*` read plus every `vi.stubEnv('VITE_*', …)` stub in
# `apps/web/src`; documentation is `apps/web/.env.example`.
#
# Rules (all offline/hermetic, key NAMES only, values never inspected):
#   1. used == declared: every VITE_* read/stubbed in src exists in env.d.ts,
#      and every declared key is actually read (no stale allowlist entries).
#   2. declared == documented: env.d.ts and .env.example carry the same
#      VITE_* set in both directions (no stale docs, no undocumented keys).
#   3. no secret segments: no underscore-delimited segment of a declared key
#      may be KEY/KEYS/SECRET/SECRETS/TOKEN/TOKENS/PASSWORD/PASSWD/PWD/
#      PRIVATE/CREDENTIAL/CREDENTIALS. A secret-looking name fails so it can
#      never be baked into the public bundle (this is the red line).
#   4. no PUBLIC_* reads in web source: Vite only exposes the VITE_
#      namespace to the browser; backend `PUBLIC_ORIGIN` is not visible to
#      frontend code (the browser reaches it through the same-origin
#      gateway, never through import.meta.env).
#   5. .env.example carries no values: active (non-comment) VITE_* lines must
#      be empty. Real build-time values come from the documented injection
#      shape only (`scripts/infisical-run.sh --env <env> -- npm run build`
#      for local/verify builds; Cloudflare Pages dashboard keeps the prod
#      overrides empty for the same-origin gateway) and are never committed.
#   6. Pages Functions allowlist: `apps/web/functions` is server-side edge
#      code whose only secret binding is `IDEABOARD_API_ORIGIN` (Cloudflare
#      Pages Function secret, populated from the Infisical value). Any other
#      `IDEABOARD_*` binding fails until the runbook blesses it.
#
# Usage: bash scripts/web-frontend-env-check.sh [repo-root]
# Exit 0 when all checks pass, 1 with a failure list otherwise.
#
# Portable to bash 3.2 (macOS /usr/bin/bash): newline-separated key lists,
# no associative arrays, no empty-array expansions.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

ROOT_DIR="${1:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

ENV_DTS="$ROOT_DIR/apps/web/src/env.d.ts"
WEB_EXAMPLE="$ROOT_DIR/apps/web/.env.example"
WEB_SRC="$ROOT_DIR/apps/web/src"
WEB_FUNCTIONS="$ROOT_DIR/apps/web/functions"

failures=0
fail() {
  printf 'web-frontend-env-check: %s\n' "$1" >&2
  failures=$((failures + 1))
}

contains_key() {
  local needle="$1"
  local list="$2"
  grep -Fxq "$needle" <<<"$list"
}

# Declared schema: `readonly VITE_*` entries in env.d.ts.
declared_keys() { grep -oE 'readonly VITE_[A-Z0-9_]*' "$ENV_DTS" | awk '{print $2}' | sort -u || true; }

# Used keys: import.meta.env.VITE_* reads plus vi.stubEnv('VITE_*', …) stubs.
# Each grep is guarded with `|| true`: under `set -o pipefail` a grep with
# no match would otherwise fail the whole command substitution silently.
used_keys() {
  {
    grep -rhoE 'import\.meta\.env\.VITE_[A-Z0-9_]*' "$WEB_SRC" 2>/dev/null | sed 's/.*\.env\.//' || true
    grep -rhoE "stubEnv\('VITE_[A-Z0-9_]*'" "$WEB_SRC" 2>/dev/null | cut -d"'" -f2 || true
  } | sort -u
  return 0
}

# Documented keys: active or commented KEY= lines in .env.example.
example_keys() { grep -oE '^[[:space:]]*#?[[:space:]]*VITE_[A-Z0-9_]*=' "$WEB_EXAMPLE" | grep -oE 'VITE_[A-Z0-9_]*' | sort -u || true; }

# Secret-looking underscore segments: any VITE_* carrying one of these would
# bake a secret into the public bundle.
is_secret_segment() {
  case "$1" in
    KEY | KEYS | SECRET | SECRETS | TOKEN | TOKENS | PASSWORD | PASSWD | PWD | PRIVATE | CREDENTIAL | CREDENTIALS) return 0 ;;
    *) return 1 ;;
  esac
}

main() {
  for file in "$ENV_DTS" "$WEB_EXAMPLE"; do
    [[ -f "$file" ]] || { fail "missing input $file"; }
  done
  [[ -d "$WEB_SRC" ]] || fail "missing input dir $WEB_SRC"
  [[ -d "$WEB_FUNCTIONS" ]] || fail "missing input dir $WEB_FUNCTIONS"
  if ((failures > 0)); then
    return 1
  fi

  local declared used documented
  declared="$(declared_keys)"
  used="$(used_keys)"
  documented="$(example_keys)"

  # 1. used == declared (both directions).
  local key
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$declared"; then
      fail "used $key in apps/web/src is not declared in apps/web/src/env.d.ts"
    fi
  done <<<"$used"
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$used"; then
      fail "declared $key in env.d.ts is never read in apps/web/src (remove it or wire it)"
    fi
  done <<<"$declared"

  # 2. declared == documented (both directions).
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$documented"; then
      fail "declared $key is not documented in apps/web/.env.example"
    fi
  done <<<"$declared"
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    if ! contains_key "$key" "$declared"; then
      fail "$WEB_EXAMPLE documents unknown key $key (not in env.d.ts)"
    fi
  done <<<"$documented"

  # 3. no secret segments in declared names (red line: every VITE_ value
  # is baked into the public bundle, so a secret-looking name fails here).
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    local rest="${key#VITE_}"
    local segment
    while IFS= read -r segment; do
      [[ -z "$segment" ]] && continue
      if is_secret_segment "$segment"; then
        fail "declared $key looks like a secret (segment '$segment'); VITE_* values are baked public, never store secrets there"
        break
      fi
    done < <(printf '%s\n' "$rest" | tr '_' '\n')
  done <<<"$declared"

  # 4. no PUBLIC_* (or other non-VITE) env reads in web source.
  local public_reads
  public_reads="$(grep -rhoE 'import\.meta\.env\.PUBLIC_[A-Z0-9_]*' "$WEB_SRC" 2>/dev/null | sort -u || true)"
  while IFS= read -r hit; do
    [[ -z "$hit" ]] && continue
    fail "web source reads $hit; Vite only exposes VITE_* to the browser (backend PUBLIC_ORIGIN is reached via the same-origin gateway)"
  done <<<"$public_reads"

  # 5. .env.example carries no committed values for VITE_*.
  while IFS= read -r line; do
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ "$line" != *=* ]] && continue
    local name="${line%%=*}"
    local value="${line#*=}"
    name="$(printf '%s' "$name" | tr -d '[:space:]')"
    [[ "$name" == VITE_* ]] || continue
    if [[ -n "${value//[[:space:]]/}" ]]; then
      fail "$WEB_EXAMPLE sets a committed value for $name; build-time values come only from the documented injection shape, never from git"
    fi
  done <"$WEB_EXAMPLE"

  # 6. Pages Functions server bindings stay on the allowlist.
  local binding
  while IFS= read -r binding; do
    [[ -z "$binding" ]] && continue
    if [[ "$binding" != "IDEABOARD_API_ORIGIN" ]]; then
      fail "apps/web/functions uses unexpected binding $binding (only IDEABOARD_API_ORIGIN is blessed; update the runbook first)"
    fi
  done < <(grep -rhoE 'IDEABOARD_[A-Z0-9_]*' "$WEB_FUNCTIONS" 2>/dev/null | sort -u || true)

  if ((failures > 0)); then
    printf 'web-frontend-env-check: %d problem(s) found\n' "$failures" >&2
    return 1
  fi
  echo "frontend env (VITE_* allowlist, no secrets, Pages binding) is in sync"
}

main "$@"
