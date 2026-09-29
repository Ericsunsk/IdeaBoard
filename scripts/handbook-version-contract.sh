#!/usr/bin/env bash
# handbook-version-contract.sh
#
# Issue #855: Developer Handbook entry points + one version source of truth.
#
# Canonical sources (defined once, never duplicated):
#   - Node.js runtime: root `.node-version`
#   - Go toolchain: `apps/api/go.mod` (`go` directive)
#
# Consumers that must stay consistent, verified here:
#   - `apps/auth/Dockerfile` (`ARG NODE_VERSION=` pin, standalone-build fallback)
#   - `apps/api/Dockerfile` (`golang:<version>-...` base image pin)
#   - CI workflows consume the canonical files via
#     `.github/actions/setup-node` / `.github/actions/setup-go`
#     (enforced by scripts/ci-workflow-contract-check.go, not here).
#   - Handbook docs (`DEVELOPMENT.md`, `README.md`,
#     `docs/runbooks/deployment.md`) reference the canonical files
#     instead of duplicating version literals.
#
# This script also verifies that every relative entry-point link in the
# handbook doc set resolves to a file or directory in the working tree.
#
# Wired into CI as the always-run `Handbook Contract` lane so version
# drift and stale entry points fail even on docs-only changes.
set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"

fail=0
deny() {
  echo "handbook-version-contract: $*" >&2
  fail=1
}

# --- 1. Node: .node-version vs auth Dockerfile pin ---
node_version="$(tr -d '[:space:]' <"$repo_root/.node-version")"
[[ -n "${node_version}" ]] || deny "root .node-version is empty"
docker_node="$(sed -n 's/^ARG NODE_VERSION=//p' "$repo_root/apps/auth/Dockerfile" | tr -d '[:space:]')"
[[ -n "${docker_node}" ]] || deny "apps/auth/Dockerfile has no ARG NODE_VERSION pin"
[[ "${docker_node}" == "${node_version}" ]] || deny "apps/auth/Dockerfile NODE_VERSION=${docker_node} != .node-version ${node_version}"

# --- 2. Go: go.mod vs api Dockerfile base image pin ---
go_version="$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$repo_root/apps/api/go.mod" | head -n 1)"
[[ -n "${go_version}" ]] || deny "apps/api/go.mod has no 'go <version>' directive"
docker_go="$(
  # shellcheck disable=SC2016 # $BUILDPLATFORM is a Dockerfile literal, must not expand here.
  sed -n 's|^FROM --platform=\$BUILDPLATFORM golang:\([0-9][0-9.]*\)-.*|\1|p' "$repo_root/apps/api/Dockerfile" | head -n 1
)"
[[ -n "${docker_go}" ]] || deny "apps/api/Dockerfile has no golang:<version> base image pin"
[[ "${docker_go}" == "${go_version}" ]] || deny "apps/api/Dockerfile golang:${docker_go} != go.mod go ${go_version}"

# --- 3. Docs reference the canonical sources instead of duplicating literals ---
require_ref() {
  local file="$1" ref="$2"
  grep -Fq "${ref}" "$repo_root/${file}" || deny "${file} must reference canonical source ${ref}"
}

require_ref "DEVELOPMENT.md" ".node-version"
require_ref "DEVELOPMENT.md" "apps/api/go.mod"
require_ref "README.md" ".node-version"
require_ref "README.md" "apps/api/go.mod"
require_ref "docs/runbooks/deployment.md" ".node-version"

# Version literals must not reappear in handbook version declarations.
# (Vulnerability-exception pins in .grype.yaml and Atlas versions are
# out of scope and live in files not listed here.)
literal_hits="$(grep -rnE '22\.[0-9]+\.[0-9]+|1\.26\.[0-9]+' \
  "$repo_root/DEVELOPMENT.md" "$repo_root/README.md" "$repo_root/docs/runbooks/deployment.md" || true)"
[[ -z "${literal_hits}" ]] || {
  deny "duplicated Node/Go version literal in handbook docs (reference .node-version / apps/api/go.mod instead):\n${literal_hits}"
}

# --- 4. Handbook entry-point links must resolve ---
handbook_docs=(
  "DEVELOPMENT.md"
  "DESIGN.md"
  "README.md"
  "AGENTS.md"
)
for runbook in "$repo_root"/docs/runbooks/*.md; do
  handbook_docs+=("docs/runbooks/$(basename "${runbook}")")
done

for doc in "${handbook_docs[@]}"; do
  doc_path="$repo_root/${doc}"
  doc_dir="$(dirname "${doc_path}")"
  [[ -f "${doc_path}" ]] || {
    deny "handbook doc missing: ${doc}"
    continue
  }
  # Markdown links plus HTML src/href used by README badges and previews.
  targets="$(
    { grep -oE '\]\(([^)]+)\)' "${doc_path}" | sed -E 's/^\]\(//; s/\)$//';
      grep -oE '(src|href)="(\./[^"]+|docs/[^"]+|apps/[^"]+|scripts/[^"]+|docker/[^"]+|deploy/[^"]+)"' "${doc_path}" | sed -E 's/^(src|href)="//; s/"$//'; } 2>/dev/null || true
  )"
  while IFS= read -r target; do
    [[ -n "${target}" ]] || continue
    case "${target}" in
      http://* | https://* | mailto:* | '#'* | /api/* | /user/* | /uploads/*) continue ;;
    esac
    # Strip anchor, query, and surrounding whitespace/quotes.
    target="${target%%#*}"
    target="${target%%\?*}"
    target="$(printf '%s' "${target}" | sed -e 's/^[[:space:]"'"'"']*//' -e 's/[[:space:]"'"'"']*$//')"
    [[ -n "${target}" ]] || continue
    case "${target}" in
      http://* | https://* | mailto:* | '#'* | /api/* | /user/* | /uploads/*) continue ;;
    esac
    resolved="$(cd "${doc_dir}" && realpath -m "${target}" 2>/dev/null || printf '%s/%s' "${doc_dir}" "${target}")"
    [[ -e "${resolved}" ]] || deny "broken handbook link in ${doc}: ${target}"
  done <<<"${targets}"
done

if [[ "${fail}" -ne 0 ]]; then
  echo >&2
  echo "handbook version/link contract violated (issue #855)." >&2
  echo "Fix the pin, reference, or link above; do not add a compatibility allowlist." >&2
  exit 1
fi

echo "handbook version/link contract: OK"
