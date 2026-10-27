#!/usr/bin/env bash
# tasking-settlement-seam-check.sh
#
# Fail-closed settlement seam guard (Option B: no semantics change).
#
# Every *InTx seam (plus the *OutboxTx outbox writer) joins the single
# PostgreSQL settlement transaction: a missing seam, a nil store, or a nil
# outbox must surface an explicit error so the caller retries (503) or
# converges as a conflict (409). A silent success or a silent unhandled
# return discards the settlement unit: the task would flip state without
# its ledger/outbox fact, or hang as bare unavailable with no retry
# directive.
#
# Banned shapes inside seam bodies (func name contains InTx or OutboxTx):
#
#   NIL-FALSE-NIL     `return nil, false, nil`
#     A boolean-result seam reports neither a fact nor an error. A claim
#     replay needs the existing claim; a cancellation replay needs the
#     terminal task. Fail closed with an explicit error instead.
#     `return nil, true, nil` is NOT banned: it is the claim-success shape
#     (ClaimSubmissionInTx reports claimed=true with no prior claim).
#
#   SILENT-NIL-NIL    `return nil, nil`
#     Nil result with nil error. Success must carry the flipped task (or a
#     tx id); nil,nil strands the caller with no fact and no error. Fail
#     closed with an explicit error instead. `return "", nil` is NOT banned:
#     it is the legitimate zero-leg marker (non-positive Capture/Release
#     legs write no ledger row; NULL tx id is the only valid zero-leg
#     marker).
#
#   NIL-GUARD-NIL     `if <seam> == nil { ... return nil }`
#     A nil store/outbox/facts guard that returns bare success. The
#     settlement would commit without its delivery fact. Fail closed with
#     ErrSettlementUnavailable (see insertTaskOutboxTx) instead. Only `if`
#     guards naming a seam word (outbox/facts/billing/store/holder/queue/
#     claim/account/quota/scope/seam/transactor/pool/tx/db) are tracked;
#     `if err == nil` and friends are ignored.
#
# Limitations: brace tracking is lexical (line comments and double-quoted
# strings are stripped first, single-line `if x == nil { return nil }`
# included). Multi-line guard conditions and braces inside rune literals
# are not tracked; keep seam guards on one line.
#
# This script is intentionally unwired from CI workflows (those are owned
# separately). scripts/run-all-self-tests.sh picks up
# scripts/tasking-settlement-seam-check-test.sh by glob, and that self-test
# runs this checker against the real tree (case 1), so the gate rides the
# ops lane without a workflow edit. The explicit bash -n / shellcheck lists
# in .github/workflows/ops.yml do need the two new files added by hand.
#
set -euo pipefail

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
tasking_dir="$repo_root/apps/api/internal/tasking"

if [ ! -d "$tasking_dir" ]; then
  echo "tasking settlement seam check: $tasking_dir not found" >&2
  exit 1
fi

fail=0
while IFS= read -r file; do
  out="$(awk '
    function is_seam(name) { return (name ~ /InTx/ || name ~ /OutboxTx/) }
    function seam_guard(line) {
      if (line !~ /if[ \t].*==[ \t]*nil/) return 0
      lc = tolower(line)
      return (lc ~ /outbox|facts|billing|store|holder|queue|claim|account|quota|scope|seam|transactor|pool|tx|db/)
    }
    {
      raw = $0
      # Lexical normalization for brace counting only.
      norm = raw
      sub(/\/\/.*$/, "", norm)
      gsub(/"[^"]*"/, "\"\"", norm)
      if (!in_func) {
        if (match(norm, /^func[ \t]+(\([^)]*\)[ \t]+)?[A-Za-z0-9_]+/)) {
          name = substr(norm, RSTART, RLENGTH)
          sub(/^func[ \t]+(\([^)]*\)[ \t]+)?/, "", name)
          if (is_seam(name)) {
            in_func = 1
            funcname = name
            depth = 0
            started = 0
            stack_n = 0
            # Fall through to brace counting below: the signature line
            # carries the opening brace, and skipping it (next) would
            # undercount depth by one and end the seam body early.
          } else {
            next
          }
        } else {
          next
        }
      }
      opens = gsub(/\{/, "{", norm)
      closes = gsub(/\}/, "}", norm)
      depth += opens - closes
      if (opens > 0) started = 1
      if (seam_guard(raw) && raw ~ /\{/) {
        stack_n++
        guard_stack[stack_n] = depth
      }
      line = raw
      sub(/\/\/.*$/, "", line)
      if (line ~ /^[ \t]*return[ \t]+nil[ \t]*,[ \t]*false[ \t]*,[ \t]*nil[ \t]*$/) {
        printf "NIL-FALSE-NIL %s:%d: %s returns nil,false,nil (fail closed: ErrSettlementUnavailable)\n", FILENAME, NR, funcname
        failed = 1
      } else if (line ~ /^[ \t]*return[ \t]+nil[ \t]*,[ \t]*nil[ \t]*$/) {
        printf "SILENT-NIL-NIL %s:%d: %s returns nil,nil (fail closed with an explicit error)\n", FILENAME, NR, funcname
        failed = 1
      } else if (line ~ /^[ \t]*return[ \t]+nil[ \t]*$/ && stack_n > 0) {
        printf "NIL-GUARD-NIL %s:%d: %s returns bare nil under a nil seam guard (fail closed: ErrSettlementUnavailable)\n", FILENAME, NR, funcname
        failed = 1
      }
      while (stack_n > 0 && guard_stack[stack_n] > depth) stack_n--
      if (started && depth <= 0) {
        in_func = 0
        stack_n = 0
      }
    }
    END { if (failed) exit 1 }
  ' "$file")" || {
    while IFS= read -r line; do printf '  %s\n' "$line" >&2; done <<< "$out"
    fail=1
  }
done < <(find "$tasking_dir" -name '*.go' | sort)

if [ "$fail" -ne 0 ]; then
  echo >&2
  echo "tasking settlement seam guard violated" >&2
  echo "Seam (*InTx/*OutboxTx) bodies must never silently discard: return an explicit" >&2
  echo "error (ErrSettlementUnavailable / state conflict) instead of nil,false,nil," >&2
  echo "nil,nil, or bare nil under a nil seam guard." >&2
  exit 1
fi

echo "tasking settlement seams: OK"
