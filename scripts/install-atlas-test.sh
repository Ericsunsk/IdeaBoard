#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf -- "$fixture"' EXIT
mkdir -p "$fixture/app" "$fixture/bin" "$fixture/tmp"
cp "$root/apps/api/install-atlas.sh" "$fixture/app/install-atlas.sh"
printf 'ARG ATLAS_VERSION=1.3.7-0a182ba-canary\n' > "$fixture/app/Dockerfile"
printf 'verified fixture binary\n' > "$fixture/payload"
digest="$(shasum -a256 "$fixture/payload" | awk '{print $1}')"
for architecture in amd64 arm64; do
  printf '%s  atlas-linux-%s-v1.3.7-0a182ba-canary\n' "$digest" "$architecture"
done > "$fixture/app/atlas-checksums"
cat > "$fixture/bin/curl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$TEST_CALLS"
[[ "${TEST_CURL_FAIL:-0}" == 0 ]] || exit 37
while [[ "$#" -gt 0 ]]; do
  if [[ "$1" == -o ]]; then cp "$TEST_PAYLOAD" "$2"; exit; fi
  shift
done
exit 99
MOCK
chmod +x "$fixture/bin/curl"
export PATH="$fixture/bin:$PATH" TEST_PAYLOAD="$fixture/payload" TEST_CALLS="$fixture/calls" TMPDIR="$fixture/tmp"
unset ATLAS_VERSION
for architecture in arm64 x86_64; do
  "$fixture/app/install-atlas.sh" "$fixture/installed" "$architecture"
  cmp "$fixture/payload" "$fixture/installed"
done
grep -Fq 'https://atlasbinaries.com/atlas/atlas-linux-arm64-v1.3.7-0a182ba-canary' "$fixture/calls"
grep -Fq 'https://atlasbinaries.com/atlas/atlas-linux-amd64-v1.3.7-0a182ba-canary' "$fixture/calls"
reject() {
  if "$@" > "$fixture/failure.log" 2>&1; then echo 'invalid Atlas install succeeded' >&2; exit 1; fi
  cmp "$fixture/payload" "$fixture/installed"
  [[ -z "$(find "$fixture/tmp" -mindepth 1 -print -quit)" ]]
}
reject "$fixture/app/install-atlas.sh" "$fixture/installed" unsupported
reject env ATLAS_VERSION='1.3.7;false' "$fixture/app/install-atlas.sh" "$fixture/installed" arm64
reject env ATLAS_VERSION=1.3.8 "$fixture/app/install-atlas.sh" "$fixture/installed" arm64
# Missing/invalid pins fail before making a network request.
[[ "$(wc -l < "$fixture/calls" | tr -d ' ')" == 2 ]]
printf corrupt > "$fixture/corrupt"
reject env TEST_PAYLOAD="$fixture/corrupt" "$fixture/app/install-atlas.sh" "$fixture/installed" arm64
reject env TEST_CURL_FAIL=1 "$fixture/app/install-atlas.sh" "$fixture/installed" arm64
cp "$fixture/app/atlas-checksums" "$fixture/checksums"
cat "$fixture/checksums" >> "$fixture/app/atlas-checksums"
reject "$fixture/app/install-atlas.sh" "$fixture/installed" arm64
echo 'Verified Atlas installer fixtures passed'
