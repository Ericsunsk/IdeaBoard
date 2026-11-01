#!/usr/bin/env bash
#
# OpenAPI breaking-change gate (issue #772).
#
# Compares each live contract spec at <base-ref> against the working tree and
# fails when oasdiff reports error-level breaking changes (removed paths or
# operations, new required request fields, removed response fields, tighter
# enums, ...). Additive changes (new optional fields, new paths) pass.
#
# Live specs (compared by default):
#   apps/api/openapi.public.yaml
#   apps/api/openapi.control.yaml
#   apps/api/openapi.events.yaml   (valid OpenAPI 3.0.3 with paths +
#     components, so oasdiff diffs it like the other two)
#
# Usage:
#   scripts/openapi-breaking-check.sh <base-ref> [head-file]
#
# An explicit [head-file] overrides the default spec list with that single
# file (keeps fixture-driven callers working). The CI lane in
# .github/workflows/api.yml passes only <base-ref>, so all live specs above
# are compared there with no wiring change.
#
# Environment:
#   OASDIFF_VERSION            pinned oasdiff release (default v1.32.1)
# Reviewed breaks are recorded in scripts/openapi-breaking-reviews.tsv with
# the exact base commit and SHA-256 of both contract snapshots. The comparison
# still runs; a record applies to that spec/range only, on PRs and pushes alike.
# PR labels never skip comparison or approve other contract changes.
#
# Exit 0: no unreviewed breaking changes (or nothing to compare). Exit 1:
# unreviewed changes, or a live head spec is missing (fail loudly, never skip).
# Other non-zero statuses are errors from oasdiff or its setup.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

OASDIFF_VERSION="${OASDIFF_VERSION:-v1.32.1}"
BASE_REF="${1:-}"
DEFAULT_SPECS=(
	apps/api/openapi.public.yaml
	apps/api/openapi.control.yaml
	apps/api/openapi.events.yaml
)
SPEC_FILES=()
if [[ $# -ge 2 && -n "${2:-}" ]]; then
	SPEC_FILES=("$2")
else
	SPEC_FILES=("${DEFAULT_SPECS[@]}")
fi
if [[ -z "${BASE_REF}" || "${BASE_REF}" =~ ^0+$ ]]; then
	echo "openapi-breaking-check: skipped (no base ref; workflow_dispatch without history)"
	exit 0
fi

# A missing live head spec is a hard gate failure, never a silent skip:
# the previous default (deleted apps/api/openapi.yaml) let this lane fail
# for the wrong reason on every run.
for spec in "${SPEC_FILES[@]}"; do
	if [[ ! -f "${spec}" ]]; then
		echo "openapi-breaking-check: head spec ${spec} is missing" >&2
		exit 1
	fi
done

snapshot_sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1"
	else
		shasum -a 256 "$1"
	fi | awk '{print $1}'
}

reviewed_break() {
	local reviews="${SCRIPT_DIR}/openapi-breaking-reviews.tsv"
	[[ -f "${reviews}" ]] || return 1
	local base_commit base_hash head_hash
	base_commit="$(git rev-parse "${BASE_REF}^{commit}")" || return 1
	base_hash="$(snapshot_sha256 "$2")" || return 1
	head_hash="$(snapshot_sha256 "$1")" || return 1
	awk -F '\t' -v spec="$1" -v commit="${base_commit}" -v before="${base_hash}" -v after="${head_hash}" '
		/^[[:space:]]*(#|$)/ { next }
		NF != 5 || $1 == "" || length($2) != 40 || $2 ~ /[^0-9a-f]/ ||
		length($3) != 64 || $3 ~ /[^0-9a-f]/ ||
		length($4) != 64 || $4 ~ /[^0-9a-f]/ || $5 == "" { invalid = 1; next }
		$1 == spec && $2 == commit && $3 == before && $4 == after { matched = 1; review = $5 }
		END {
			if (invalid) { print "openapi-breaking-check: invalid review record" > "/dev/stderr"; exit 1 }
			if (!matched) exit 1
			print review
		}
	' "${reviews}"
}

workdir="$(mktemp -d)"
cleanup_path "${workdir}"

gobin="${workdir}/bin"
mkdir -p "${gobin}"
GOBIN="${gobin}" go install "github.com/oasdiff/oasdiff@${OASDIFF_VERSION}"

breaking_specs=()
for spec in "${SPEC_FILES[@]}"; do
	if ! git cat-file -e "${BASE_REF}:${spec}" 2>/dev/null; then
		echo "openapi-breaking-check: base ${BASE_REF} has no ${spec}; nothing to compare"
		continue
	fi

	if git diff --quiet "${BASE_REF}" -- "${spec}"; then
		echo "openapi-breaking-check: ${spec} unchanged since ${BASE_REF}"
		continue
	fi

	base_file="${workdir}/$(basename "${spec}").base.yaml"
	git show "${BASE_REF}:${spec}" >"${base_file}"

	echo "openapi-breaking-check: ${BASE_REF}:${spec} -> working tree (${spec})"
	status=0
	"${gobin}/oasdiff" breaking --fail-on ERR "${base_file}" "${spec}" || status=$?
	case "${status}" in
		0)
			echo "openapi-breaking-check: no breaking changes in ${spec}"
			;;
		1)
			if review="$(reviewed_break "${spec}" "${base_file}")"; then
				echo "openapi-breaking-check: reviewed breaking changes in ${spec} (${review})"
			else
				echo "openapi-breaking-check: BREAKING changes detected in ${spec}" >&2
				breaking_specs+=("${spec}")
			fi
			;;
		*)
			echo "openapi-breaking-check: oasdiff failed on ${spec} with exit status ${status}" >&2
			exit "${status}"
			;;
	esac
done

if [[ "${#breaking_specs[@]}" -gt 0 ]]; then
	echo ""
	echo "openapi-breaking-check: BREAKING OpenAPI changes detected in: ${breaking_specs[*]}" >&2
	echo "Keep the contract backward compatible, or obtain explicit review of the" >&2
	echo "exact base commit and contract snapshots in scripts/openapi-breaking-reviews.tsv." >&2
	exit 1
fi

echo "openapi-breaking-check: no unreviewed breaking changes"
