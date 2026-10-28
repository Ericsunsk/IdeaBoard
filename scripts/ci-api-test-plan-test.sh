#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"
go test scripts/ci-api-test-plan.go scripts/ci-api-test-plan_test.go
