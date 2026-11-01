#!/usr/bin/env bash
# ai-proxy-dev-vars.sh
#
# 从 Infisical dev 灌入 Worker Env 的全部本地 bindings（wrangler dev 读取）。
#
#   先 infisical login，再：
#   scripts/ai-proxy-dev-vars.sh
#
# 输出固定：apps/ai-proxy/.dev.vars（0600，仅含 src/contracts.ts 的 Env keys）。
# 任一 binding 缺失、重复或值为空则报错退出，保留旧文件。
# 值来源唯一：infisical export --env=dev --format=dotenv --path /ai-proxy
# （Worker 四件套）。root `/` 不再是来源：残留的 WORKER_INTERNAL_SECRET root
# 副本待用户在 dashboard 手动删除（删 key 是破坏性操作，脚本永不删）；删除前
# 每次灌入都以 /ai-proxy 为准对 root 副本做一致性校验（只比哈希，不打印值），
# 不一致则 loud fail，保留旧文件。
#
# Portable to bash 3.2 (macOS /usr/bin/bash): no assoc arrays, no mapfile.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=scripts/lib/env-schema.sh
source "$SCRIPT_DIR/lib/env-schema.sh"
OUT_FILE="${REPO_ROOT}/apps/ai-proxy/.dev.vars"

usage() {
  cat <<'EOF'
usage: scripts/ai-proxy-dev-vars.sh
  先 infisical login，再跑本脚本；输出固定 apps/ai-proxy/.dev.vars（0600）
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi
if (($# > 0)); then
  usage >&2
  exit 2
fi

command -v infisical >/dev/null 2>&1 || {
  echo "error: infisical CLI 未安装（npm i -g @infisical/cli；再 infisical login）" >&2
  exit 1
}

# Temporary output shares the target directory so the final rename is atomic.
umask 077
cloud_tmp="$(mktemp "${TMPDIR:-/tmp}/ai-proxy-infisical.XXXXXX")"
trap 'rm -f "$cloud_tmp" "${err_tmp:-}" "${out_tmp:-}" "${root_tmp:-}"' EXIT
err_tmp="$(mktemp "${TMPDIR:-/tmp}/ai-proxy-infisical-error.XXXXXX")"
out_tmp="$(mktemp "${OUT_FILE}.tmp.XXXXXX")"
root_tmp="$(mktemp "${TMPDIR:-/tmp}/ai-proxy-infisical-root.XXXXXX")"

worker_path="$(worker_binding_source_path)"
export_args=(export --env=dev --format=dotenv --path "$worker_path")
if [[ -n "${INFISICAL_PROJECT_ID:-}" ]]; then
  export_args+=(--projectId "${INFISICAL_PROJECT_ID}")
fi
if [[ -n "${INFISICAL_DOMAIN:-}" ]]; then
  export_args+=(--domain "${INFISICAL_DOMAIN}")
fi
if ! infisical "${export_args[@]}" >"$cloud_tmp" 2>"$err_tmp"; then
  cat "$err_tmp" >&2
  exit 1
fi

# Root 副本待用户 dashboard 删除前的过渡读取：只用于一致性校验，不作值来源。
root_args=(export --env=dev --format=dotenv --path /)
if [[ -n "${INFISICAL_PROJECT_ID:-}" ]]; then
  root_args+=(--projectId "${INFISICAL_PROJECT_ID}")
fi
if [[ -n "${INFISICAL_DOMAIN:-}" ]]; then
  root_args+=(--domain "${INFISICAL_DOMAIN}")
fi
if ! infisical "${root_args[@]}" >"$root_tmp" 2>"$err_tmp"; then
  cat "$err_tmp" >&2
  exit 1
fi

# Keep dotenv quoting intact, never source an export as shell code.
keys="$(worker_binding_keys "$REPO_ROOT/apps/ai-proxy/src/contracts.ts" worker)"
if [[ -z "$keys" ]]; then
  echo "error: Worker Env binding schema 为空，未写入 ${OUT_FILE}" >&2
  exit 1
fi
missing=0
while IFS= read -r key; do
  lines="$(grep -E "^${key}=" "$cloud_tmp" || true)"
  count="$(printf '%s\n' "$lines" | grep -c -E "^${key}=" || true)"
  if [[ "$count" != 1 ]]; then
    echo "error: Infisical dev 的 ${key} 必须恰有一个值（未写入 ${OUT_FILE}）" >&2
    missing=1
    continue
  fi
  val="${lines#*=}"
  # Bindings are single-line tokens/IDs; strip matching dotenv quotes only
  # for validation, preserving the original line for Wrangler's parser.
  case "$val" in
    \"*\" | \'*\') stripped="${val:1:${#val}-2}" ;;
    \"* | \'*)
      echo "error: Infisical dev 的 ${key} 必须是单行值（未写入 ${OUT_FILE}）" >&2
      missing=1
      continue
      ;;
    *) stripped="$val" ;;
  esac
  if [[ -z "${stripped//[[:space:]]/}" ]]; then
    echo "error: Infisical dev 的 ${key} 为空（未写入 ${OUT_FILE}）" >&2
    missing=1
    continue
  fi
  printf '%s\n' "$lines" >>"$out_tmp"
done <<<"$keys"
if ((missing != 0)); then
  exit 1
fi

# Matching root copies are migration-only; the shared module owns this policy.
worker_binding_root_copies_match "$REPO_ROOT/apps/ai-proxy/src/contracts.ts" "$root_tmp" "$cloud_tmp"

chmod 600 "$out_tmp"
mv "$out_tmp" "$OUT_FILE"
echo "wrote ${OUT_FILE} (0600, Worker Env bindings)"
