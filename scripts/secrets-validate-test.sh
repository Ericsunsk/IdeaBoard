#!/usr/bin/env bash
# secrets-validate-test.sh
#
# Self-test for scripts/secrets-validate.sh. Builds synthetic secret bundles
# under a temp fixture dir and asserts the validator accepts a well-formed
# bundle while rejecting each corruption class. Requires jq only (no sops,
# no network); this is the highest-risk secrets script with zero coverage.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

fixture_dir="$(mktemp -d)"
cleanup_path "$fixture_dir"

validator="$SCRIPT_DIR/secrets-validate.sh"
b64_32="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

write_bundle() {
  local file="$1"
  local mutate="$2"
  jq -n \
    --arg postgres "0123456789abcdef0123456789abcdef" \
    --arg apiDb "1123456789abcdef0123456789abcdef" \
    --arg authDb "2123456789abcdef0123456789abcdef" \
    --arg internal "3123456789abcdef0123456789abcdef" \
    --arg encryption "$b64_32" \
    --arg credits "$b64_32" \
    --arg rewards '{"current":{"version":"v1","key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},"previous":null}' \
    --arg authSecret "4123456789abcdef0123456789abcdef" \
    --arg nats "abcdefghij0123456789ABCDEFGH._~-" \
    --arg imgKey "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
    --arg imgSalt "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" \
    '{
      POSTGRES_PASSWORD: $postgres,
      API_DB_PASSWORD: $apiDb,
      AUTH_DB_PASSWORD: $authDb,
      AUTH_INTERNAL_KEY: $internal,
      APP_SECRETS_ENCRYPTION_KEY: $encryption,
      CREDITS_WELCOME_IDENTITY_KEY: $credits,
      REWARDS_CODE_HMAC_KEYS: $rewards,
      BETTER_AUTH_SECRET: $authSecret,
      NATS_PASSWORD: $nats,
      IMGPROXY_KEY: $imgKey,
      IMGPROXY_SALT: $imgSalt
    }' | jq "$mutate" >"$file"
}

failures=0
expect_pass() {
  local name="$1"
  local file="$2"
  if "$validator" "$file" >/dev/null 2>&1; then
    echo "ok: $name"
  else
    echo "FAIL (expected pass): $name" >&2
    failures=$((failures + 1))
  fi
}
expect_fail() {
  local name="$1"
  local file="$2"
  local message="$3"
  local output
  if output="$("$validator" "$file" 2>&1)"; then
    echo "FAIL (expected rejection): $name" >&2
    failures=$((failures + 1))
  elif grep -Fq "$message" <<<"$output"; then
    echo "ok: $name"
  else
    echo "FAIL (wrong error for $name): $output" >&2
    failures=$((failures + 1))
  fi
}

valid="$fixture_dir/valid.json"
write_bundle "$valid" "."
expect_pass "well-formed bundle is valid" "$valid"

missing="$fixture_dir/missing.json"
write_bundle "$missing" "del(.AUTH_INTERNAL_KEY)"
expect_fail "missing key is rejected" "$missing" "missing or empty secret: AUTH_INTERNAL_KEY"

short="$fixture_dir/short.json"
write_bundle "$short" '.POSTGRES_PASSWORD = "too-short"'
expect_fail "short password is rejected" "$short" "POSTGRES_PASSWORD must be at least 32 characters"

same_proxy="$fixture_dir/same-proxy.json"
write_bundle "$same_proxy" '.IMGPROXY_SALT = .IMGPROXY_KEY'
expect_fail "identical imgproxy key/salt is rejected" "$same_proxy" "IMGPROXY_KEY and IMGPROXY_SALT must be different"

bad_b64="$fixture_dir/bad-b64.json"
write_bundle "$bad_b64" '.APP_SECRETS_ENCRYPTION_KEY = "not-base64!!"'
expect_fail "bad encryption key is rejected" "$bad_b64" "APP_SECRETS_ENCRYPTION_KEY must be base64 that decodes to exactly 32 bytes"

bad_hmac="$fixture_dir/bad-hmac.json"
write_bundle "$bad_hmac" '.REWARDS_CODE_HMAC_KEYS = "not-json"'
expect_fail "non-JSON HMAC keys are rejected" "$bad_hmac" "REWARDS_CODE_HMAC_KEYS must contain a JSON object"

bad_nats="$fixture_dir/bad-nats.json"
write_bundle "$bad_nats" '.NATS_PASSWORD = "has space and /slash!!!!!!!"'
expect_fail "non-URL-safe NATS password is rejected" "$bad_nats" "NATS_PASSWORD must use URL-safe characters"

if ((failures > 0)); then
  echo "secrets-validate contract tests failed: $failures case(s)" >&2
  exit 1
fi
echo "secrets-validate contract tests passed"
