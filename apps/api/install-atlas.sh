#!/usr/bin/env bash
# One verified Atlas binary for image builds and migration verification.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ "$#" -ge 1 && "$#" -le 2 ]] || { echo 'usage: install-atlas.sh destination [architecture]' >&2; exit 1; }
destination="$1"
architecture="${2:-$(uname -m)}"
case "$architecture" in
  x86_64|amd64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo "unsupported Atlas architecture: $architecture" >&2; exit 1 ;;
esac
version="${ATLAS_VERSION:-$(sed -n 's/^ARG ATLAS_VERSION=//p' "$script_dir/Dockerfile" | tr -d '[:space:]')}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9a-f]{7,40}-canary)?$ ]] || { echo 'invalid Atlas version pin' >&2; exit 1; }
filename="atlas-linux-${architecture}-v${version}"
expected="$(awk -v name="$filename" '$2 == name {print $1}' "$script_dir/atlas-checksums")"
[[ "$expected" =~ ^[0-9a-f]{64}$ ]] || { echo "missing unique Atlas checksum: $filename" >&2; exit 1; }
temporary="$(mktemp -d)"
trap 'rm -rf -- "$temporary"' EXIT
curl --fail --silent --show-error --location --retry 2 --retry-all-errors \
  --connect-timeout 15 --max-time 180 \
  "https://atlasbinaries.com/atlas/$filename" -o "$temporary/atlas"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/atlas" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$temporary/atlas" | awk '{print $1}')"
fi
[[ "$actual" == "$expected" ]] || { echo "Atlas checksum mismatch: $filename" >&2; exit 1; }
install -m 0555 "$temporary/atlas" "$destination"
