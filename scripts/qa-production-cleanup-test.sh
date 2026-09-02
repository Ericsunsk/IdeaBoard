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
  *'/api/user/info'*) printf '%s\n' '{"data":{"user":{"public_id":"00000000-0000-0000-0000-000000000001"}}}' ;;
  *'/api/user/boards'*) printf '%s\n' '{"data":{"list":[{"board_id":"qa-board-1","name":"QA qa-20260831-abc123"}]}}' ;;
  *'/api/user/library/assets?limit=100'*) printf '%s\n' '{"data":{"items":[{"id":7,"title":"QA qa-20260831-abc123 source"}]}}' ;;
  *) printf '%s\n' '{"data":{"deleted":true}}' ;;
esac
EOF
chmod +x "$fake_bin/curl"
cookie="$test_root/cookies"; touch "$cookie"
run_cleanup() {
  PATH="$fake_bin:$PATH" QA_TEST_REQUESTS="$requests" QA_COOKIE_FILE="$cookie" \
    QA_OWNER_PUBLIC_ID="${QA_OWNER_PUBLIC_ID:-00000000-0000-0000-0000-000000000001}" QA_TEST_RUN_ID="${QA_TEST_RUN_ID:-qa-20260831-abc123}" \
    QA_API_BASE_URL='https://api.example.test' PUBLIC_ORIGIN='https://ideaboard.test' "$script_dir/qa-production-cleanup.sh" "$@"
}
run_cleanup_with_public_origin() {
  PATH="$fake_bin:$PATH" QA_TEST_REQUESTS="$requests" QA_COOKIE_FILE="$cookie" \
    QA_OWNER_PUBLIC_ID="${QA_OWNER_PUBLIC_ID:-00000000-0000-0000-0000-000000000001}" QA_TEST_RUN_ID="${QA_TEST_RUN_ID:-qa-20260831-abc123}" \
    QA_API_BASE_URL='' PUBLIC_ORIGIN='https://ideaboard.test' "$script_dir/qa-production-cleanup.sh" "$@"
}
output="$(run_cleanup --board-id qa-board-1 --library-asset-id 7)"
grep -Fq 'DRY-RUN: no resources deleted' <<<"$output"
if grep -Fq -- '--request DELETE' "$requests"; then echo 'dry-run issued DELETE' >&2; exit 1; fi
: >"$requests"
output="$(run_cleanup --board-id qa-board-1 --library-asset-id 7 --confirm)"
grep -Fq 'DELETED BOARD qa-board-1' <<<"$output"
grep -Fq 'DELETED ASSET 7' <<<"$output"
grep -Fq -- '--request DELETE' "$requests"
if run_cleanup --board-id other-board >/dev/null 2>&1; then echo 'marker guard failed' >&2; exit 1; fi
if QA_OWNER_PUBLIC_ID='00000000-0000-0000-0000-000000000002' run_cleanup --board-id qa-board-1 >/dev/null 2>&1; then echo 'owner guard failed' >&2; exit 1; fi
if QA_TEST_RUN_ID='production-test' run_cleanup --board-id qa-board-1 >/dev/null 2>&1; then echo 'run-id validation failed' >&2; exit 1; fi
: >"$requests"
run_cleanup_with_public_origin --board-id qa-board-1 >/dev/null
grep -Fq 'https://ideaboard.test/api/user/info' "$requests"
echo 'QA production cleanup contract tests passed'
