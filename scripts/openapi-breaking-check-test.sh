#!/usr/bin/env bash
# Exercise the actual gate with isolated Git history and a hermetic oasdiff
# process. Review matching never replaces or short-circuits that comparison.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime; common.sh is linted separately.
source "$SCRIPT_DIR/lib/common.sh"
fixture_dir="$(mktemp -d)"
cleanup_path "$fixture_dir"
passed=0

expect_pass() {
  local name="$1"; shift
  if ! "$@" > "$fixture_dir/$name.log" 2>&1; then
    cat "$fixture_dir/$name.log" >&2
    echo "FAIL: $name unexpectedly failed" >&2
    exit 1
  fi
  passed=$((passed + 1))
}

expect_rejected() {
  local name="$1" expected="$2"; shift 2
  if "$@" > "$fixture_dir/$name.log" 2>&1; then
    echo "FAIL: $name unexpectedly passed" >&2
    exit 1
  fi
  if ! grep -Fq "$expected" "$fixture_dir/$name.log"; then
    cat "$fixture_dir/$name.log" >&2
    echo "FAIL: $name reported the wrong failure" >&2
    exit 1
  fi
  passed=$((passed + 1))
}

checker="$SCRIPT_DIR/openapi-breaking-check.sh"
expect_pass empty-base bash "$checker" ''
expect_pass zero-base bash "$checker" 0000000000000000000000000000000000000000
OPENAPI_BREAKING_LABELS='openapi-breaking-approved' \
  expect_rejected label-missing-head 'is missing' bash "$checker" HEAD "$fixture_dir/absent.yaml"

mkdir -p "$fixture_dir/repo/scripts/lib" "$fixture_dir/repo/apps/api" "$fixture_dir/bin"
cp "$checker" "$fixture_dir/repo/scripts/"
cp "$SCRIPT_DIR/lib/common.sh" "$fixture_dir/repo/scripts/lib/"
checker="$fixture_dir/repo/scripts/openapi-breaking-check.sh"
export OASDIFF_FIXTURE="$fixture_dir/oasdiff" OASDIFF_TEST_LOG="$fixture_dir/compared.log"
cat > "$OASDIFF_FIXTURE" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == breaking && "$2" == --fail-on && "$3" == ERR ]]
printf '%s\n' "$5" >> "$OASDIFF_TEST_LOG"
if grep -q '^# TOOL_ERROR' "$5"; then exit 3; fi
if grep -q '^# BREAKING' "$5"; then exit 1; fi
MOCK
cat > "$fixture_dir/bin/go" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == install && "$2" == github.com/oasdiff/oasdiff@* ]]
cp "$OASDIFF_FIXTURE" "$GOBIN/oasdiff"
chmod +x "$GOBIN/oasdiff"
MOCK
chmod +x "$fixture_dir/bin/go"
export PATH="$fixture_dir/bin:$PATH"
export GIT_AUTHOR_NAME=Fixture GIT_AUTHOR_EMAIL=fixture@example.invalid
export GIT_COMMITTER_NAME=Fixture GIT_COMMITTER_EMAIL=fixture@example.invalid
git -C "$fixture_dir/repo" init -q
for name in public control events; do
  printf 'openapi: 3.0.3\ninfo: {title: Fixture, version: "1"}\npaths: {}\n' \
    > "$fixture_dir/repo/apps/api/openapi.$name.yaml"
done
git -C "$fixture_dir/repo" add apps
git -C "$fixture_dir/repo" commit -qm 'verified baseline'
base="$(git -C "$fixture_dir/repo" rev-parse HEAD)"
cp "$fixture_dir/repo/apps/api/openapi.control.yaml" "$fixture_dir/base-control.yaml"
review_file="$fixture_dir/repo/scripts/openapi-breaking-reviews.tsv"

run_checker() {
  (cd "$fixture_dir/repo" && bash "$checker" "$@")
}

fingerprint() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi | awk '{print $1}'
}

write_review() {
  printf 'apps/api/openapi.control.yaml\t%s\t%s\t%s\t2026-11-01:fixture-review\n' \
    "$base" "$(fingerprint "$fixture_dir/base-control.yaml")" \
    "$(fingerprint "$fixture_dir/repo/apps/api/openapi.control.yaml")" > "$review_file"
}

expect_pass unchanged run_checker "$base"
[[ ! -f "$OASDIFF_TEST_LOG" ]]
printf '# BREAKING reviewed removal\n' >> "$fixture_dir/repo/apps/api/openapi.control.yaml"
cp "$fixture_dir/repo/apps/api/openapi.control.yaml" "$fixture_dir/reviewed-control.yaml"
expect_rejected no-record 'BREAKING changes detected' run_checker "$base"
OPENAPI_BREAKING_LABELS='openapi-breaking-approved' \
  expect_rejected label-only 'BREAKING changes detected' run_checker "$base"

write_review
: > "$OASDIFF_TEST_LOG"
GITHUB_EVENT_NAME=pull_request OPENAPI_BREAKING_LABELS='openapi-breaking-approved' \
  expect_pass reviewed-pr run_checker "$base"
GITHUB_EVENT_NAME=push OPENAPI_BREAKING_LABELS='' \
  expect_pass reviewed-push run_checker "$base"
[[ "$(wc -l < "$OASDIFF_TEST_LOG" | tr -d ' ')" == 2 ]]
grep -Fq 'reviewed breaking changes' "$fixture_dir/reviewed-push.log"

# A different trusted commit is not an approval, even with identical base YAML.
printf 'Later baseline.\n' > "$fixture_dir/repo/README.md"
git -C "$fixture_dir/repo" add README.md
git -C "$fixture_dir/repo" commit -qm 'another verified commit'
other_base="$(git -C "$fixture_dir/repo" rev-parse HEAD)"
expect_rejected wrong-base 'BREAKING changes detected' run_checker "$other_base"

printf 'invalid record\n' > "$review_file"
expect_rejected malformed-record 'invalid review record' run_checker "$base"
write_review
awk -F '\t' 'BEGIN { OFS = "\t" } { $3 = sprintf("%064d", 0); print }' "$review_file" > "$review_file.tmp"
mv "$review_file.tmp" "$review_file"
expect_rejected wrong-base-snapshot 'BREAKING changes detected' run_checker "$base"
write_review
awk -F '\t' 'BEGIN { OFS = "\t" } { $4 = sprintf("%064d", 0); print }' "$review_file" > "$review_file.tmp"
mv "$review_file.tmp" "$review_file"
expect_rejected wrong-head-snapshot 'BREAKING changes detected' run_checker "$base"
write_review
printf 'invalid trailing record\n' >> "$review_file"
expect_rejected malformed-after-match 'invalid review record' run_checker "$base"

write_review
printf '# BREAKING extra unreviewed change\n' >> "$fixture_dir/repo/apps/api/openapi.control.yaml"
expect_rejected extra-control-break 'BREAKING changes detected' run_checker "$base"
cp "$fixture_dir/reviewed-control.yaml" "$fixture_dir/repo/apps/api/openapi.control.yaml"
for name in public events; do
  printf '# BREAKING unreviewed contract\n' >> "$fixture_dir/repo/apps/api/openapi.$name.yaml"
  expect_rejected "unreviewed-$name" "BREAKING changes detected in apps/api/openapi.$name.yaml" run_checker "$base"
  cp "$fixture_dir/base-control.yaml" "$fixture_dir/repo/apps/api/openapi.$name.yaml"
done

printf '# TOOL_ERROR\n' >> "$fixture_dir/repo/apps/api/openapi.control.yaml"
write_review
expect_rejected tool-error 'oasdiff failed' run_checker "$base"
cp "$fixture_dir/base-control.yaml" "$fixture_dir/repo/apps/api/openapi.control.yaml"
printf '# additive optional field\n' >> "$fixture_dir/repo/apps/api/openapi.control.yaml"
rm "$review_file"
expect_pass additive-no-record run_checker "$base"
rm "$fixture_dir/repo/apps/api/openapi.events.yaml"
OPENAPI_BREAKING_LABELS='openapi-breaking-approved' \
  expect_rejected missing-live-spec 'head spec apps/api/openapi.events.yaml is missing' run_checker "$base"

echo "openapi-breaking-check contract tests passed ($passed cases)"
