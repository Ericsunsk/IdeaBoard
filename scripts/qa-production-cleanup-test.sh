#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT
fake_bin="$test_root/bin"; mkdir -p "$fake_bin"
requests="$test_root/requests"; : >"$requests"
cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${QA_TEST_REQUESTS}"
case "$*" in
  *'/api/v1/user'*) printf '%s\n' '{"data":{"id":"usr_test0001"}}' ;;
  *'/api/v1/workspaces/ws_test0001/boards'*) printf '%s\n' '{"data":[{"id":"board_qa1","name":"QA qa-20260831-abc123"}],"meta":{"next_cursor":null}}' ;;
  *'/api/v1/workspaces/ws_test0001/library/assets'*) printf '%s\n' '{"data":[{"id":"asset_qa7","title":"QA qa-20260831-abc123 source"}],"meta":{"next_cursor":null}}' ;;
  *) printf '%s\n' '{"data":{"archived":true}}' ;;
esac
EOF
chmod +x "$fake_bin/curl"
cookie="$test_root/cookies"; touch "$cookie"
run_cleanup() {
  PATH="$fake_bin:$PATH" QA_TEST_REQUESTS="$requests" QA_COOKIE_FILE="$cookie" \
    QA_OWNER_USER_ID="${QA_OWNER_USER_ID:-usr_test0001}" QA_WORKSPACE_ID="${QA_WORKSPACE_ID:-ws_test0001}" QA_TEST_RUN_ID="${QA_TEST_RUN_ID:-qa-20260831-abc123}" \
    QA_API_BASE_URL='https://api.example.test' PUBLIC_ORIGIN='https://ideaboard.test' "$script_dir/qa-production-cleanup.sh" "$@"
}
run_cleanup_with_public_origin() {
  PATH="$fake_bin:$PATH" QA_TEST_REQUESTS="$requests" QA_COOKIE_FILE="$cookie" \
    QA_OWNER_USER_ID="${QA_OWNER_USER_ID:-usr_test0001}" QA_WORKSPACE_ID="${QA_WORKSPACE_ID:-ws_test0001}" QA_TEST_RUN_ID="${QA_TEST_RUN_ID:-qa-20260831-abc123}" \
    QA_API_BASE_URL='' PUBLIC_ORIGIN='https://ideaboard.test' "$script_dir/qa-production-cleanup.sh" "$@"
}
output="$(run_cleanup --board-id board_qa1 --library-asset-id asset_qa7)"
grep -Fq 'DRY-RUN: no resources deleted' <<<"$output"
if grep -Fq -- '--request' "$requests"; then echo 'dry-run issued write' >&2; exit 1; fi
: >"$requests"
output="$(run_cleanup --board-id board_qa1 --library-asset-id asset_qa7 --confirm)"
grep -Fq 'ARCHIVED BOARD board_qa1' <<<"$output"
grep -Fq 'DELETED ASSET asset_qa7' <<<"$output"
grep -Fq -- '--request POST' "$requests"
grep -Fq -- 'Idempotency-Key:' "$requests"
grep -Fq -- '--request DELETE' "$requests"
if run_cleanup --board-id other-board >/dev/null 2>&1; then echo 'marker guard failed' >&2; exit 1; fi
if QA_OWNER_USER_ID='usr_test0002' run_cleanup --board-id board_qa1 >/dev/null 2>&1; then echo 'owner guard failed' >&2; exit 1; fi
if QA_TEST_RUN_ID='production-test' run_cleanup --board-id board_qa1 >/dev/null 2>&1; then echo 'run-id validation failed' >&2; exit 1; fi
if QA_WORKSPACE_ID='not-a-workspace' run_cleanup --board-id board_qa1 >/dev/null 2>&1; then echo 'workspace validation failed' >&2; exit 1; fi
: >"$requests"
run_cleanup_with_public_origin --board-id board_qa1 >/dev/null
grep -Fq 'https://ideaboard.test/api/v1/user' "$requests"
echo 'QA production cleanup contract tests passed'
