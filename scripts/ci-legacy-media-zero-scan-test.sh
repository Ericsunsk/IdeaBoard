#!/usr/bin/env bash
# Self-test for the clean-break scan. It uses isolated trees so the test
# remains meaningful while the repository is intentionally mid-cutover.

set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
scan="$script_dir/ci-legacy-media-zero-scan.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$tmp_dir/clean/apps/api" "$tmp_dir/clean/apps/web/src"
if REPO_ROOT="$tmp_dir/clean" bash "$scan" >/dev/null; then
  echo 'OK: clean tree passes'
else
  echo 'FAIL: clean tree should pass' >&2
  exit 1
fi

mkdir -p "$tmp_dir/legacy/apps/api/internal/workflow" "$tmp_dir/legacy/apps/web/src"
printf '%s\n' 'const retired = "original_image_url"' >"$tmp_dir/legacy/apps/api/internal/workflow/bad.go"
if REPO_ROOT="$tmp_dir/legacy" bash "$scan" >/dev/null 2>&1; then
  echo 'FAIL: legacy production contract should fail' >&2
  exit 1
fi

mkdir -p "$tmp_dir/frontend/apps/api" "$tmp_dir/frontend/apps/web/src"
printf '%s\n' 'const uploadURL = "/uploads/task.png"' >"$tmp_dir/frontend/apps/web/src/bad.ts"
if REPO_ROOT="$tmp_dir/frontend" bash "$scan" >/dev/null 2>&1; then
  echo 'FAIL: legacy frontend URL contract should fail' >&2
  exit 1
fi

echo 'OK: legacy production contracts fail'
