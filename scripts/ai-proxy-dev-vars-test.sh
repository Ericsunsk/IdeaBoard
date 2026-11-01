#!/usr/bin/env bash
# Hermetic local binding setup contract: fake Infisical, isolated output only.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime; common.sh is linted directly.
source "$SCRIPT_DIR/lib/common.sh"

fixture_root="$(mktemp -d)"
cleanup_path "$fixture_root"
mkdir -p "$fixture_root/scripts/lib" "$fixture_root/apps/ai-proxy/src" "$fixture_root/bin" "$fixture_root/tmp"
cp "$SCRIPT_DIR/ai-proxy-dev-vars.sh" "$fixture_root/scripts/"
cp "$SCRIPT_DIR/lib/env-schema.sh" "$fixture_root/scripts/lib/"
cp "$SCRIPT_DIR/../apps/ai-proxy/src/contracts.ts" "$fixture_root/apps/ai-proxy/src/"
cat >"$fixture_root/bin/infisical" <<'FAKE'
#!/usr/bin/env bash
printf '%s\n' "$@" >>"$FAKE_ARGS"
if [[ "${FAKE_FAILURE:-0}" == 1 ]]; then
  echo "fake export failed" >&2
  exit 1
fi
is_ai_proxy=0
for a in "$@"; do
  if [[ "$a" == "/ai-proxy" ]]; then
    is_ai_proxy=1
  fi
done
if ((is_ai_proxy == 1)); then
  cat "$FAKE_EXPORT"
else
  cat "${FAKE_ROOT_EXPORT:-$FAKE_EXPORT}"
fi
FAKE
chmod +x "$fixture_root/bin/infisical"
export PATH="$fixture_root/bin:$PATH"
export TMPDIR="$fixture_root/tmp"
export FAKE_ARGS="$fixture_root/args"
export FAKE_EXPORT="$fixture_root/export.env"
export INFISICAL_PROJECT_ID="fixture-project"
export INFISICAL_DOMAIN="https://fixture.invalid"

cat >"$fixture_root/valid.env" <<'VALUES'
CF_ACCOUNT_ID="fixture-account"
CF_GATEWAY_ID='fixture-gateway'
CF_AIG_TOKEN="fixture-token"
WORKER_INTERNAL_SECRET="fixture-worker-token"
UNRELATED_SECRET="must-not-be-copied"
VALUES
helper="$fixture_root/scripts/ai-proxy-dev-vars.sh"
output="$fixture_root/apps/ai-proxy/.dev.vars"
cp "$fixture_root/valid.env" "$FAKE_EXPORT"
# Root 副本默认与 /ai-proxy 一致（过渡期常态）。
export FAKE_ROOT_EXPORT="$fixture_root/root.env"
cp "$fixture_root/valid.env" "$FAKE_ROOT_EXPORT"
bash "$helper" >"$fixture_root/log" 2>&1
for key in CF_ACCOUNT_ID CF_GATEWAY_ID CF_AIG_TOKEN WORKER_INTERNAL_SECRET; do
  line="$(grep "^${key}=" "$FAKE_EXPORT")"
  grep -Fxq "$line" "$output" || die "binding changed: $key"
done
[[ "$(wc -l <"$output" | tr -d ' ')" == 4 ]] || die "unexpected bindings copied"
[[ "$(find "$output" -perm 0600 -print)" == "$output" ]] || die "output permissions differ from 0600"
grep -Fxq -- --env=dev "$FAKE_ARGS" || die "wrong export environment"
grep -Fxq -- /ai-proxy "$FAKE_ARGS" || die "primary export must read /ai-proxy"
grep -Fxq -- fixture-project "$FAKE_ARGS" || die "project override missing"
grep -Fxq -- https://fixture.invalid "$FAKE_ARGS" || die "domain override missing"
cp "$output" "$fixture_root/previous.env"

expect_rejection() {
  if bash "$helper" >"$fixture_root/log" 2>&1; then
    die "expected rejection: $1"
  fi
  cmp -s "$output" "$fixture_root/previous.env" || die "overwrote old bindings: $1"
  [[ -z "$(find "$fixture_root/apps/ai-proxy" -name '.dev.vars.tmp.*' -print)" ]] || die "left output temporary file"
  [[ -z "$(find "$TMPDIR" -type f -print)" ]] || die "left export/error temporary file"
}

for key in CF_ACCOUNT_ID CF_GATEWAY_ID CF_AIG_TOKEN WORKER_INTERNAL_SECRET; do
  grep -v "^${key}=" "$fixture_root/valid.env" >"$FAKE_EXPORT"
  expect_rejection "missing $key"
  for empty in '' '""' "''" '"   "'; do
    grep -v "^${key}=" "$fixture_root/valid.env" >"$FAKE_EXPORT"
    printf '%s=%s\n' "$key" "$empty" >>"$FAKE_EXPORT"
    expect_rejection "empty $key"
  done
done
cp "$fixture_root/valid.env" "$FAKE_EXPORT"
printf 'CF_AIG_TOKEN="duplicate"\n' >>"$FAKE_EXPORT"
expect_rejection "duplicate binding"
cp "$fixture_root/valid.env" "$FAKE_EXPORT"
export FAKE_FAILURE=1
expect_rejection "export failed"
unset FAKE_FAILURE

# Root 副本一致性：root 已删完（无该 key）直接过；root 值不同则 loud fail。
cp "$fixture_root/valid.env" "$FAKE_EXPORT"
grep -v "^WORKER_INTERNAL_SECRET=" "$fixture_root/valid.env" >"$FAKE_ROOT_EXPORT"
bash "$helper" >"$fixture_root/log" 2>&1 || die "deleted root copy should pass"
cp "$output" "$fixture_root/previous.env"
grep -v "^WORKER_INTERNAL_SECRET=" "$fixture_root/valid.env" >"$FAKE_ROOT_EXPORT"
printf 'WORKER_INTERNAL_SECRET="stale-root-copy"\n' >>"$FAKE_ROOT_EXPORT"
expect_rejection "root copy mismatch"
cp "$fixture_root/valid.env" "$FAKE_ROOT_EXPORT"

# Every stale Worker binding at root obeys the same migration consistency rule.
for key in CF_ACCOUNT_ID CF_GATEWAY_ID CF_AIG_TOKEN; do
  grep -v "^${key}=" "$fixture_root/valid.env" >"$FAKE_ROOT_EXPORT"
  printf '%s="stale-root-copy"\n' "$key" >>"$FAKE_ROOT_EXPORT"
  expect_rejection "root copy mismatch for $key"
done
cp "$fixture_root/valid.env" "$FAKE_ROOT_EXPORT"

# The export is data, never sourced as shell code.
cp "$fixture_root/valid.env" "$FAKE_EXPORT"
printf 'UNRELATED_SECRET=%s(touch "%s")\n' '$' "$fixture_root/executed" >>"$FAKE_EXPORT"
bash "$helper" >"$fixture_root/log" 2>&1
[[ ! -e "$fixture_root/executed" ]] || die "executed export content"
[[ -z "$(find "$TMPDIR" -type f -print)" ]] || die "success left temporary file"
echo "ai-proxy local binding contract tests passed"
