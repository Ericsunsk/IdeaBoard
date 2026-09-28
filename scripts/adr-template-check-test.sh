#!/usr/bin/env bash
# adr-template-check-test.sh
#
# Self-test for scripts/adr-template-check.sh. Builds synthetic ADR trees
# under a temp ADR_DIR and asserts the exit code matches expectations.
# This is what the ops contract lane should invoke; without it, a bug in the
# template checker would silently pass.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/adr-template-check.sh"

write_adr() {
  local dir="$1"
  local name="$2"
  shift 2
  {
    echo "# ADR-0000: Test"
    echo
    for heading in "$@"; do
      echo "## $heading"
      echo
      echo "Body."
      echo
    done
  } > "$dir/$name"
}

# Case 1: the real tree passes (scaffolding README/TEMPLATE excluded).
if ADR_DIR="$script_dir/../docs/adr" bash "$checker" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Case 2: each missing heading must fail.
for missing in Context Decision Consequences Status; do
  rm -f "$tmp"/*.md
  headings=()
  for h in Context Decision Consequences Status; do
    [[ "$h" != "$missing" ]] || continue
    headings+=("$h")
  done
  write_adr "$tmp" "0001-test.md" "${headings[@]}"
  if ADR_DIR="$tmp" bash "$checker" >/dev/null 2>&1; then
    echo "FAIL: missing ## $missing should fail" >&2
    exit 1
  fi
  echo "OK: missing ## $missing fails"
done

# Case 3: scaffolding files are ignored (README/TEMPLATE without headings).
rm -f "$tmp"/*.md
write_adr "$tmp" "0001-test.md" Context Decision Consequences Status
printf '# Not an ADR\n' > "$tmp/README.md"
printf '# Template\n' > "$tmp/TEMPLATE.md"
if ADR_DIR="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: scaffolding ignored"
else
  echo "FAIL: scaffolding should be ignored" >&2
  exit 1
fi

# Case 4: an empty ADR directory fails loudly, never skips.
rm -f "$tmp"/*.md
if ADR_DIR="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: empty ADR dir should fail" >&2
  exit 1
fi
echo "OK: empty ADR dir fails"

# Case 5: a missing ADR directory fails loudly.
if ADR_DIR="$tmp/does-not-exist" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: missing ADR dir should fail" >&2
  exit 1
fi
echo "OK: missing ADR dir fails"

# Case 6: the grandfather allowlist is per-file, not per-heading. A NEW file
# missing Status must fail even though 0001 is grandfathered for Status.
rm -f "$tmp"/*.md
write_adr "$tmp" "0099-new.md" Context Decision Consequences
if ADR_DIR="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: new file missing Status should fail" >&2
  exit 1
fi
echo "OK: allowlist does not leak to new files"

echo "adr-template-check self-test: OK"
