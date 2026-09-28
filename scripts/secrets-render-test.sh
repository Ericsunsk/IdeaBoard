#!/usr/bin/env bash
# secrets-render-test.sh
#
# Self-test for scripts/secrets-render.sh. Uses a fake `sops` on PATH whose
# --decrypt passthrough cats the bundle, so no age keys or network are needed.
# jq is real. Cases: a valid bundle renders sorted KEY=value lines with mode
# 600; an invalid bundle fails without creating output; a missing bundle fails.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

fixture_dir="$(mktemp -d)"
cleanup_path "$fixture_dir"

fake_bin="$fixture_dir/bin"
mkdir -p "$fake_bin"
cat >"$fake_bin/sops" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
# Fake sops: support only the decrypt passthrough used by secrets-render.sh.
if [[ "${1:-}" == "--decrypt" && -n "${2:-}" ]]; then
  cat "$2"
  exit 0
fi
echo "fake sops: unsupported invocation: $*" >&2
exit 1
EOF
chmod +x "$fake_bin/sops"

bundle="$fixture_dir/bundle.sops.json"
jq -n \
  --arg postgres "0123456789abcdef0123456789abcdef" \
  --arg apiDb "1123456789abcdef0123456789abcdef" \
  --arg authDb "2123456789abcdef0123456789abcdef" \
  --arg internal "3123456789abcdef0123456789abcdef" \
  --arg encryption "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" \
  --arg credits "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" \
  --arg rewards '{"current":{"version":"v1","key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},"previous":null}' \
  --arg authSecret "4123456789abcdef0123456789abcdef" \
  --arg nats "abcdefghij0123456789ABCDEFGH._~-x" \
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
  }' >"$bundle"

output="$fixture_dir/rendered.env"
PATH="$fake_bin:$PATH" bash "$SCRIPT_DIR/secrets-render.sh" "$bundle" "$output" >/dev/null
[[ -f "$output" ]] || { echo "FAIL: rendered env file was not created" >&2; exit 1; }

# Sorted by key, raw KEY=value lines, restrictive permissions.
first_key="$(head -n 1 "$output" | cut -d= -f1)"
[[ "$first_key" == "API_DB_PASSWORD" ]] || { echo "FAIL: output is not key-sorted (first: $first_key)" >&2; exit 1; }
grep -Fq "AUTH_INTERNAL_KEY=3123456789abcdef0123456789abcdef" "$output" || {
  echo "FAIL: rendered values do not match the bundle" >&2
  exit 1
}
mode="$(stat -f %Lp "$output" 2>/dev/null || stat -c %a "$output")"
[[ "$mode" == "600" ]] || { echo "FAIL: rendered env mode is $mode, want 600" >&2; exit 1; }

# An invalid bundle must fail before any output is created.
bad_bundle="$fixture_dir/bad.sops.json"
jq -n '{POSTGRES_PASSWORD: "x"}' >"$bad_bundle"
bad_output="$fixture_dir/bad.env"
if PATH="$fake_bin:$PATH" bash "$SCRIPT_DIR/secrets-render.sh" "$bad_bundle" "$bad_output" >/dev/null 2>&1; then
  echo "FAIL: invalid bundle rendered without error" >&2
  exit 1
fi
[[ ! -e "$bad_output" ]] || { echo "FAIL: output created for invalid bundle" >&2; exit 1; }

# A missing bundle must fail fast.
if PATH="$fake_bin:$PATH" bash "$SCRIPT_DIR/secrets-render.sh" "$fixture_dir/absent.json" "$fixture_dir/absent.env" >/dev/null 2>&1; then
  echo "FAIL: missing bundle rendered without error" >&2
  exit 1
fi

echo "secrets-render contract tests passed"
