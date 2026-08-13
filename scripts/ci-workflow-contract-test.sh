#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

(cd "${root}/apps/api" && go run ../../scripts/ci-workflow-contract-check.go)
