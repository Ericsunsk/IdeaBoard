#!/usr/bin/env bash
# ci-legacy-media-zero-scan.sh
#
# Shipping code and frontend configuration must use typed Media contracts.

set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
fail=0

scan_source() {
  local label="$1"
  local pattern="$2"
  shift 2
  local file
  while IFS= read -r -d '' file; do
    printf '✗ %s: %s\n' "$label" "${file#"$repo_root"/}" >&2
    fail=1
  done < <(
    if command -v rg >/dev/null 2>&1; then
      rg -l -0 --glob '*.go' --glob '*.ts' --glob '*.tsx' \
        --glob '!**/*_test.go' --glob '!**/*.spec.ts' --glob '!**/*.unit.spec.ts' \
        --glob '!**/node_modules/**' --glob '!**/dist/**' \
        -- "$pattern" "$@" 2>/dev/null || true
    else
      # CI may lack rg. Match its source scope with grep rather than skip
      # scanning; unlike rg -0, this fallback cannot preserve newline paths.
      grep -rl \
        --include='*.go' --include='*.ts' --include='*.tsx' \
        --exclude='*_test.go' --exclude='*.spec.ts' --exclude='*.unit.spec.ts' \
        --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=.git \
        -e "$pattern" -- "$@" 2>/dev/null | tr '\n' '\0' || true
    fi
  )
}

# No durable task/provider contract may keep the retired original URL
# parameter. This includes generated client code: the API contract must
# be regenerated after the field is removed.
scan_source 'retired original_image_url contract' 'original_image_url' \
  "$repo_root/apps/api" "$repo_root/apps/web/src"

# Browser delivery is same-origin MediaRef delivery. A public asset-base
# configuration would recreate the deprecated cross-origin URL path.
scan_source 'retired VITE_PUBLIC_ASSET_BASE_URL configuration' 'VITE_PUBLIC_ASSET_BASE_URL' \
  "$repo_root/apps/web/src" "$repo_root/apps/web/.env" "$repo_root/apps/web/.env.example"

# Frontend business code must receive a typed MediaRef rather than build
# the retired static upload URL itself.
scan_source 'retired /uploads/ frontend URL contract' '"/uploads/' \
  "$repo_root/apps/web/src"

# Workflow and worker production code may only cross into Media through
# typed ports. Tests are excluded by scan_source's *_test.go rule.
scan_source 'forbidden concrete media/storage dependency' 'ideaboard/api/internal/media/storage' \
  "$repo_root/apps/api/internal/runtime/interfaces/worker" "$repo_root/apps/api/internal/workflow"

if ((fail != 0)); then
  cat >&2 <<'EOF'

media contract scan: FAIL
Use typed Media contracts at the ownership boundary. See DESIGN.md §7.
EOF
  exit 1
fi

echo 'media contract scan: OK'
