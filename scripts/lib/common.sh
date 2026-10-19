#!/usr/bin/env bash
# scripts/lib/common.sh
#
# Shared shell helpers. This file is SOURCED, never executed:
#   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#   # shellcheck disable=SC1091 # Resolved at runtime; this file is linted directly in ops.yml.
#   source "$SCRIPT_DIR/lib/common.sh"
#
# Sourcing is side-effect free: functions are defined, no trap is installed
# until cleanup_path is called, and nothing is printed. Re-sourcing is a
# no-op via the guard below.
#
# Helpers (dominant-repo-idiom consolidation):
#   die <message>           print "Error: <message>" to stderr, exit 1.
#                           Matches the deploy-stack.sh / uninstall-vps.sh
#                           idiom byte-for-byte for plain messages (printf
#                           %s, so no echo backslash/flag surprises).
#                           Scripts with custom die contracts keep their own:
#                           qa-production-cleanup.sh (qa-cleanup: prefix,
#                           exit 2) and production-deploy-entrypoint.sh
#                           (JSON envelope) intentionally do NOT use this.
#   log <message>           print "<message>" to stdout (info channel).
#   require_env VAR...      die unless every named variable is set and
#                           non-empty. Message: "Error: <VAR> is required".
#   cleanup_path <path>...  remove each path when the script exits (EXIT
#                           trap). Empty paths are skipped; missing paths are
#                           fine (rm -rf semantics). The trap is installed
#                           once no matter how often cleanup_path is called.
#                           Matches the render-nginx-template.sh /
#                           render-cloudflare-real-ip-conf.sh
#                           idiom: temp outputs that were moved into place are
#                           simply absent at cleanup time (no-op).
#
# Portable to bash 3.2 (macOS /usr/bin/bash): no assoc arrays, no mapfile.

# Guard: sourcing twice must not reset registered cleanup paths.
if [[ -n "${IDEABOARD_COMMON_SH_LOADED:-}" ]]; then
  return 0
fi
IDEABOARD_COMMON_SH_LOADED=1

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

log() {
  printf '%s\n' "$*"
}

require_env() {
  local missing=0
  local name
  for name in "$@"; do
    if [[ -z "${!name:-}" ]]; then
      printf 'Error: %s is required\n' "$name" >&2
      missing=1
    fi
  done
  if ((missing != 0)); then
    exit 1
  fi
}

_IDEABOARD_CLEANUP_PATHS=()
_IDEABOARD_CLEANUP_TRAP_INSTALLED=0

# shellcheck disable=SC2317,SC2329 # Invoked indirectly by the EXIT trap installed in cleanup_path.
_ideaboard_cleanup_run() {
  if ((${#_IDEABOARD_CLEANUP_PATHS[@]} == 0)); then
    return 0
  fi
  local path
  for path in "${_IDEABOARD_CLEANUP_PATHS[@]}"; do
    [[ -n "$path" ]] || continue
    rm -rf -- "$path"
  done
}

cleanup_path() {
  local path
  for path in "$@"; do
    _IDEABOARD_CLEANUP_PATHS+=("$path")
  done
  if ((_IDEABOARD_CLEANUP_TRAP_INSTALLED == 0)); then
    _IDEABOARD_CLEANUP_TRAP_INSTALLED=1
    trap _ideaboard_cleanup_run EXIT
  fi
}
