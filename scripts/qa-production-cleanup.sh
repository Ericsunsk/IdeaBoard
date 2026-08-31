#!/usr/bin/env bash
set -euo pipefail

# Remove only explicitly marked QA resources through authenticated APIs.
# Dry-run is the default; media objects are reclaimed by the existing worker.
api_base_url="${QA_API_BASE_URL:-${PUBLIC_ORIGIN:?Set PUBLIC_ORIGIN or QA_API_BASE_URL}}"
cookie_file="${QA_COOKIE_FILE:-}"
owner_public_id="${QA_OWNER_PUBLIC_ID:-}"
run_id="${QA_TEST_RUN_ID:-}"
confirm=0
board_ids=()
library_asset_ids=()

usage() {
  cat >&2 <<'EOF'
Usage: QA_COOKIE_FILE=/path/cookies QA_OWNER_PUBLIC_ID=<uuid> QA_TEST_RUN_ID=qa-YYYYMMDD-token scripts/qa-production-cleanup.sh [--board-id ID] [--library-asset-id ID] [--confirm]
The command is dry-run by default. --confirm is required for DELETE requests.
EOF
}
die() { printf 'qa-cleanup: %s\n' "$*" >&2; exit 2; }

while (($#)); do
  case "$1" in
    --board-id) (($# >= 2)) || die "--board-id requires a value"; board_ids+=("$2"); shift 2 ;;
    --library-asset-id) (($# >= 2)) || die "--library-asset-id requires a value"; library_asset_ids+=("$2"); shift 2 ;;
    --confirm) confirm=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ -f "$cookie_file" ]] || die "QA_COOKIE_FILE must point to a cookie file"
[[ "$owner_public_id" =~ ^[0-9a-fA-F-]{36}$ ]] || die "QA_OWNER_PUBLIC_ID must be a UUID"
[[ "$run_id" =~ ^qa-[0-9]{8}-[a-z0-9-]{6,48}$ ]] || die "QA_TEST_RUN_ID must match qa-YYYYMMDD-<token>"
(( ${#board_ids[@]} + ${#library_asset_ids[@]} > 0 )) || die "at least one explicit resource ID is required"

curl_api() { curl --fail-with-body --silent --show-error --cookie "$cookie_file" -H 'Accept: application/json' "$@"; }
info_json="$(curl_api "$api_base_url/api/user/info")"
actual_owner="$(jq -r '.data.user.public_id // .user.public_id // empty' <<<"$info_json")"
[[ "$actual_owner" == "$owner_public_id" ]] || die "owner guard failed (session is $actual_owner)"
boards_json="$(curl_api "$api_base_url/api/user/boards")"
assets_json="$(curl_api "$api_base_url/api/user/library/assets?limit=100")"

if (( ${#board_ids[@]} > 0 )); then
  for id in "${board_ids[@]}"; do
    name="$(jq -r --arg id "$id" '.data.list[]? | select(.board_id == $id) | .name // empty' <<<"$boards_json")"
    [[ -n "$name" ]] || die "board $id is not visible to the authenticated owner"
    [[ "$name" == *"$run_id"* ]] || die "board $id marker guard failed"
    printf 'BOARD %s %s\n' "$id" "$name"
  done
fi
if (( ${#library_asset_ids[@]} > 0 )); then
  for id in "${library_asset_ids[@]}"; do
    row="$(jq -c --arg id "$id" '.data.items[]? | select((.id|tostring) == $id)' <<<"$assets_json")"
    [[ -n "$row" ]] || die "library asset $id is not visible to the authenticated owner"
    marker_text="$(jq -r '[.title, .source_id, .author] | map(select(. != null)) | join(" ")' <<<"$row")"
    [[ "$marker_text" == *"$run_id"* ]] || die "library asset $id marker guard failed"
    printf 'ASSET %s %s\n' "$id" "$(jq -r '.title // .source_id // "(untitled)"' <<<"$row")"
  done
fi

if (( ! confirm )); then
  echo 'DRY-RUN: no resources deleted. Re-run with --confirm after reviewing the list.'
  exit 0
fi
if (( ${#board_ids[@]} > 0 )); then
  for id in "${board_ids[@]}"; do
    curl_api --request DELETE "$api_base_url/api/user/boards/$id" >/dev/null
    printf 'DELETED BOARD %s\n' "$id"
  done
fi
if (( ${#library_asset_ids[@]} > 0 )); then
  for id in "${library_asset_ids[@]}"; do
    curl_api --request DELETE "$api_base_url/api/user/library/assets/$id" >/dev/null
    printf 'DELETED ASSET %s\n' "$id"
  done
fi
echo 'Cleanup complete. Media objects remain under the reclamation worker grace period.'
