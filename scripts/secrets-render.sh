#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  echo "Usage: $0 <sops-bundle.json> <output.env>" >&2
}

if [[ $# -ne 2 ]]; then
  usage
  exit 2
fi

bundle_file="$1"
output_file="$2"
command -v sops >/dev/null 2>&1 || { echo "sops is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
[[ -f "$bundle_file" ]] || { echo "secret bundle does not exist: $bundle_file" >&2; exit 1; }

work_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

decrypted_file="$work_dir/secrets.json"
sops --decrypt "$bundle_file" > "$decrypted_file"
bash "$(dirname "$0")/secrets-validate.sh" "$decrypted_file" >/dev/null

tmp_output="$(mktemp "${output_file}.XXXXXX")"
cleanup_output() {
  rm -f "$tmp_output"
}
trap cleanup_output EXIT

# Bundle values are generated without whitespace/newlines, so raw dotenv values
# preserve the exact secret bytes when Compose parses the rendered file.
jq -r 'to_entries | sort_by(.key)[] | "\(.key)=\(.value)"' "$decrypted_file" > "$tmp_output"
chmod 600 "$tmp_output"
mv "$tmp_output" "$output_file"
trap - EXIT
trap cleanup EXIT

echo "rendered secret env file: $output_file"
