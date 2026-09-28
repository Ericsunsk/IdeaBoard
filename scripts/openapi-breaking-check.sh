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
#   OPENAPI_BREAKING_LABELS    comma-joined PR labels; the exact label
#                              openapi-breaking-approved makes the gate pass
#                              a note (intentional contract breaks stay
#                              reviewable instead of silently blocked)
#
# Exit 0: no breaking changes (or nothing to compare). Exit 1: breaking
# changes, or a live head spec is missing (fail loudly, never skip).
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
APPROVED_LABEL="openapi-breaking-approved"
PR_LABELS=()
if [[ -n "${OPENAPI_BREAKING_LABELS:-}" ]]; then
	IFS=',' read -r -a PR_LABELS <<<"${OPENAPI_BREAKING_LABELS}"
fi
APPROVAL_PRESENT=false
if [[ -n "${OPENAPI_BREAKING_LABELS:-}" ]]; then
	for label in "${PR_LABELS[@]}"; do
		label="${label#"${label%%[![:space:]]*}"}"
		label="${label%"${label##*[![:space:]]}"}"
		if [[ "${label}" == "${APPROVED_LABEL}" ]]; then
			APPROVAL_PRESENT=true
			break
		fi
	done
fi

if [[ "${APPROVAL_PRESENT}" == true ]]; then
	echo "openapi-breaking-check: skipped (${APPROVED_LABEL} label present)"
	exit 0
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
			echo "openapi-breaking-check: BREAKING changes detected in ${spec}" >&2
			breaking_specs+=("${spec}")
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
	echo "If the break is intentional, have a reviewer add the '${APPROVED_LABEL}'" >&2
	echo "label and re-run; otherwise keep the contract backward compatible" >&2
	echo "(additive paths/optional fields only)." >&2
	exit 1
fi

echo "openapi-breaking-check: no breaking changes"
