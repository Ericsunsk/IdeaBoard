#!/usr/bin/env bash
# adr-template-check.sh
#
# ADR template guard.
#
# docs/adr/TEMPLATE.md defines four required sections (## Context,
# ## Decision, ## Consequences, ## Status). A numbered ADR missing one of
# them is either unfinished or diverged from the template, and reviewers
# should not have to catch that by eye. The smallest change that kills the
# drift class is this grep guard: every numbered docs/adr/[0-9]*-*.md file
# must contain all four headings.
#
# Scaffolding is excluded: docs/adr/README.md and docs/adr/TEMPLATE.md are
# not ADRs (see docs/adr/README.md), and only [0-9]*-*.md files are checked.
#
# Zero numbered ADR files is a hard failure, never a silent skip.
#
set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
adr_dir="${ADR_DIR:-$repo_root/docs/adr}"

if [[ ! -d "$adr_dir" ]]; then
  echo "adr-template-check: ADR directory $adr_dir is missing" >&2
  exit 1
fi

required_headings=(
  "Context"
  "Decision"
  "Consequences"
  "Status"
)

# Grandfathered (file, heading) pairs, with reasons. Keep this list minimal:
# adding a line here weakens the guard for that file. New ADR files fail
# closed on ANY missing heading; existing files fail closed on any missing
# heading NOT listed here.
heading_allowlist=(
  # Pre-Status-convention ADRs: full Context/Decision/Consequences sections,
  # decided before ## Status was part of the skeleton. Do not retrofit history.
  "0001-postgresql-credits-ledger.md:Status"
  "0003-payments-ddd-package-boundaries.md:Status"
  "0004-no-projection-gateway.md:Status"
  # Metadata-style records: status/owner live in header lines (0005) or YAML
  # front matter (0006), predating the four-heading skeleton. Rewriting these
  # records into the skeleton is a docs edit, not this guard's job.
  "0005-frontend-ddd-reorganization.md:Context"
  "0005-frontend-ddd-reorganization.md:Decision"
  "0005-frontend-ddd-reorganization.md:Consequences"
  "0005-frontend-ddd-reorganization.md:Status"
  "0006-board-optimistic-revision-concurrency.md:Context"
  "0006-board-optimistic-revision-concurrency.md:Decision"
  "0006-board-optimistic-revision-concurrency.md:Consequences"
  "0006-board-optimistic-revision-concurrency.md:Status"
)

fail=0
checked=0

for adr in "$adr_dir"/[0-9]*-*.md; do
  # No match expands to the literal pattern when nullglob is off; skip it.
  [[ -f "$adr" ]] || continue
  checked=$((checked + 1))
  missing=()
  base="$(basename "$adr")"
  for heading in "${required_headings[@]}"; do
    if ! grep -qE "^## $heading([[:space:]]|$)" "$adr"; then
      key="$base:$heading"
      excused=0
      for entry in "${heading_allowlist[@]}"; do
        if [[ "$key" == "$entry" ]]; then
          excused=1
          break
        fi
      done
      if ((excused == 0)); then
        missing+=("$heading")
      fi
    fi
  done
  if ((${#missing[@]} > 0)); then
    echo "MISSING-HEADING $base: ${missing[*]}" >&2
    fail=1
  else
    echo "OK: $base"
  fi
done

if ((checked == 0)); then
  echo "adr-template-check: no numbered ADR files in $adr_dir" >&2
  exit 1
fi

if ((fail != 0)); then
  echo >&2
  echo "ADR template headings missing (need: ## ${required_headings[*]})" >&2
  echo "Copy the section skeleton from docs/adr/TEMPLATE.md." >&2
  exit 1
fi

echo "adr template: OK ($checked files)"
