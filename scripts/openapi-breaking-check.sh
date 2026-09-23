#!/usr/bin/env bash
#
# OpenAPI breaking-change gate (issue #772).
#
# Compares <head-file> at <base-ref> against the working tree and
# fails when oasdiff reports error-level breaking changes (removed paths or
# operations, new required request fields, removed response fields, tighter
# enums, ...). Additive changes (new optional fields, new paths) pass.
#
# Usage:
#   scripts/openapi-breaking-check.sh <base-ref> [head-file]
#
# Environment:
#   OASDIFF_VERSION            pinned oasdiff release (default v1.32.1)
#   OPENAPI_BREAKING_LABELS    comma-joined PR labels; the exact label
#                              openapi-breaking-approved makes the gate pass
#                              a note (intentional contract breaks stay
#                              reviewable instead of silently blocked)
#
# Exit 0: no breaking changes (or nothing to compare). Exit 1: breaking.
# Other non-zero statuses are errors from oasdiff or its setup.

set -euo pipefail

OASDIFF_VERSION="${OASDIFF_VERSION:-v1.32.1}"
BASE_REF="${1:-}"
HEAD_FILE="${2:-apps/api/openapi.yaml}"
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

if [[ ! -f "${HEAD_FILE}" ]]; then
	echo "openapi-breaking-check: head spec ${HEAD_FILE} is missing" >&2
	exit 1
fi

if ! git cat-file -e "${BASE_REF}:${HEAD_FILE}" 2>/dev/null; then
	echo "openapi-breaking-check: base ${BASE_REF} has no ${HEAD_FILE}; nothing to compare"
	exit 0
fi

if git diff --quiet "${BASE_REF}" -- "${HEAD_FILE}"; then
	echo "openapi-breaking-check: ${HEAD_FILE} unchanged since ${BASE_REF}"
	exit 0
fi

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT
git show "${BASE_REF}:${HEAD_FILE}" >"${workdir}/base.yaml"

gobin="${workdir}/bin"
mkdir -p "${gobin}"
GOBIN="${gobin}" go install "github.com/oasdiff/oasdiff@${OASDIFF_VERSION}"

echo "openapi-breaking-check: ${BASE_REF}:${HEAD_FILE} -> working tree (${HEAD_FILE})"
status=0
"${gobin}/oasdiff" breaking --fail-on ERR "${workdir}/base.yaml" "${HEAD_FILE}" || status=$?
case "${status}" in
	0)
		echo "openapi-breaking-check: no breaking changes"
		;;
	1)
	echo ""
	echo "openapi-breaking-check: BREAKING OpenAPI changes detected." >&2
	echo "If the break is intentional, have a reviewer add the '${APPROVED_LABEL}'" >&2
	echo "label and re-run; otherwise keep the contract backward compatible" >&2
	echo "(additive paths/optional fields only)." >&2
		exit 1
		;;
	*)
		echo "openapi-breaking-check: oasdiff failed with exit status ${status}" >&2
		exit "${status}"
		;;
esac
