#!/usr/bin/env bash
# secrets-init-test.sh
#
# Self-test for scripts/secrets-init.sh. Uses a fake `sops` on PATH whose
# --encrypt passthrough cats its input file, so no age recipient keys are
# needed; openssl is real. Cases: init creates a 600 bundle that passes
# secrets-validate.sh; a missing SOPS_AGE_RECIPIENT fails fast.
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
# Fake sops: support only the file-encrypt passthrough used by secrets-init.sh.
if [[ "${1:-}" == "--encrypt" ]]; then
  cat "${@: -1}"
  exit 0
fi
echo "fake sops: unsupported invocation: $*" >&2
exit 1
EOF
chmod +x "$fake_bin/sops"

bundle="$fixture_dir/secrets.sops.json"
SOPS_AGE_RECIPIENT="age1testrecipient000000000000000000000000000000000" \
  PATH="$fake_bin:$PATH" \
  bash "$SCRIPT_DIR/secrets-init.sh" "$bundle" >/dev/null
[[ -f "$bundle" ]] || { echo "FAIL: encrypted bundle was not created" >&2; exit 1; }

mode="$(stat -f %Lp "$bundle" 2>/dev/null || stat -c %a "$bundle")"
[[ "$mode" == "600" ]] || { echo "FAIL: bundle mode is $mode, want 600" >&2; exit 1; }

# The fake sops passthrough leaves plaintext JSON, which must satisfy the
# same validator the init script runs before encrypting.
bash "$SCRIPT_DIR/secrets-validate.sh" "$bundle" >/dev/null || {
  echo "FAIL: generated bundle fails secrets-validate.sh" >&2
  exit 1
}

# Generated secrets must be unique per run (no fixed fixtures in the script).
second="$fixture_dir/second.sops.json"
SOPS_AGE_RECIPIENT="age1testrecipient000000000000000000000000000000000" \
  PATH="$fake_bin:$PATH" \
  bash "$SCRIPT_DIR/secrets-init.sh" "$second" >/dev/null
[[ "$(jq -r .POSTGRES_PASSWORD "$bundle")" != "$(jq -r .POSTGRES_PASSWORD "$second")" ]] || {
  echo "FAIL: generated secrets are not random per run" >&2
  exit 1
}

# Missing recipient fails before writing anything.
if SOPS_AGE_RECIPIENT="" PATH="$fake_bin:$PATH" bash "$SCRIPT_DIR/secrets-init.sh" "$fixture_dir/none.json" >/dev/null 2>&1; then
  echo "FAIL: init without SOPS_AGE_RECIPIENT succeeded" >&2
  exit 1
fi

echo "secrets-init contract tests passed"
