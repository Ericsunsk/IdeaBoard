#!/usr/bin/env bash
# check-backend-copy.sh
#
# Backend copy vocabulary guard.
#
# Backend user-facing copy lives in the centralized tables
# (apps/api/internal/platform/errorrender/copy*.go, see docs/adr/0017-backend-copy-centralization.md
# plus its runtimehttp-kernel follow-up; the scan still covers the legacy
# apps/api/internal/**/message.go and apps/api/internal/**/errors.go names
# for transitional or skipped packages). The runtimehttp kernel table
# (apps/api/internal/runtime/interfaces/http/errors.go) is alias-only and
# must stay free of string literals (pinned by the alias guard below);
# wording lives exactly once in copy*.go.
# wording is managed centrally in the "Backend Copy
# Vocabulary" section of apps/api/CONTEXT.md. This script is the
# mechanical pin: every banned variant below must never reappear
# as a double-quoted literal in those tables.
#
# Rules:
#   Every "VARIANT" entry is matched byte-exact as "<VARIANT>"
#   (opening AND closing double-quote anchored, fixed-string).
#   The quotes matter: several banned forms are strict prefixes
#   of their canonical replacement ("Stripe 账户不可用" vs
#   "当前 Stripe 账户不可用", "暂不可" vs "暂不可用",
#   "读取模型映射失败" vs "读取网关模型映射失败"), so an
#   unanchored match would flag the canonical wording itself.
#
# Variants list lives in THIS script (single source; mirrors the
# scripts/openapi-pattern-check.sh canonical-table idiom). To add
# a convergence entry, extend banned_variants below AND the
# CONTEXT.md vocabulary table in the same change.
#
# To wire it, mirror the sibling contract lanes: invoke
# scripts/check-backend-copy-test.sh from
# .github/workflows/ops.yml ("Verify CI orchestration
# contracts"), and add both scripts to the bash -n / shellcheck
# lists in that file. (scripts/run-all-self-tests.sh picks up the
# self-test via its *-test.sh glob with no workflow edit.)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 # Resolved at runtime via SCRIPT_DIR; scripts/lib/common.sh is linted directly in ops.yml.
source "$SCRIPT_DIR/lib/common.sh"

repo_root="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"

# Banned variant -> canonical (comment). Keep in sync with the
# "Backend Copy Vocabulary" table in apps/api/CONTEXT.md.
banned_variants=(
  '搜索服务暂时不可用'            # -> 搜索服务暂不可用
  '视觉搜索暂时不可用'            # -> 视觉搜索暂不可用
  '当前 Stripe 服务暂时不可用'     # -> 当前 Stripe 服务暂不可用
  'Stripe 服务暂时不可用'         # -> Stripe 服务暂不可用
  '账本写入暂时失败'              # -> 账本写入失败
  '请求参数错误'                  # -> 参数错误
  'page_size 非法'                # -> page_size 无效
  '分页游标非法'                  # -> 分页游标无效
  '视觉搜索输入非法'              # -> 视觉搜索输入无效
  '操作频繁，请稍后重试'          # -> 操作过于频繁，请稍后重试
  '供应商返回为空响应'            # -> 供应商返回空响应
  '认证服务不可用'                # -> 认证服务暂不可用
  '当前工作区钱包受限，暂不可'    # -> 当前工作区钱包受限，暂不可用
  'Stripe API Key 权限不足'       # -> 当前 Stripe API Key 权限不足
  'Stripe 账户不可用'             # -> 当前 Stripe 账户不可用
  'Stripe 账户暂不具备收款能力'   # -> 当前 Stripe 账户暂不具备收款能力
  '系统设置服务未初始化'          # -> 系统设置服务未配置
  '读取模型映射失败'              # -> 读取网关模型映射失败
  '请先同意当前服务条款与退款政策' # -> 请先同意服务条款与退款政策
  '幂等请求与原创建参数不一致'    # -> 幂等请求与原请求不一致
  '状态值无效，仅支持0或1'        # -> status 仅支持 0 或 1
  '角色值无效，仅支持 admin 或 user' # -> role 仅支持 admin 或 user
)

tables=()
while IFS= read -r table; do
  tables+=("$table")
done < <(find "$repo_root/apps/api/internal" -type f \( -name 'message.go' -o -name 'errors.go' -o -name 'copy*.go' \) | sort)
(( ${#tables[@]} > 0 )) || die "no message.go/errors.go/copy*.go tables found under $repo_root/apps/api/internal"

fail=0
hits=0
for variant in "${banned_variants[@]}"; do
  pattern="\"${variant}\""
  while IFS= read -r hit; do
    [[ -n "$hit" ]] || continue
    printf 'BANNED-COPY %s (variant %q)\n' "$hit" "$variant" >&2
    fail=1
    hits=$((hits + 1))
  done < <(grep -F -n -- "$pattern" "${tables[@]}" || true)
done

if ((fail != 0)); then
  printf '\nDeprecated backend copy reappeared (%d hit(s))\n' "$hits" >&2
  printf 'Replace with the canonical wording from the "Backend Copy Vocabulary" table in apps/api/CONTEXT.md.\n' >&2
  exit 1
fi

# The runtimehttp kernel table converged into errorrender/copy.go
# (ADR-0017 follow-up): it must hold alias-only consts
# (MsgX = errorrender.MsgY) and no string literals. Comment lines,
# the package/import preamble, and the const block delimiters are
# exempt; every other line must be a bare alias assignment.
kernel_aliases="$repo_root/apps/api/internal/runtime/interfaces/http/errors.go"
if [[ -f "$kernel_aliases" ]]; then
  kernel_leftover="$(sed -e 's|//.*$||' "$kernel_aliases" \
    | grep -v '^[[:space:]]*$' \
    | grep -v '^[[:space:]]*package ' \
    | grep -v '^[[:space:]]*import' \
    | grep -v '"ideaboard' \
    | grep -v '^[[:space:]]*const' \
    | grep -v '^[[:space:]]*)$' \
    | grep -Ev '^[[:space:]]*Msg[A-Za-z0-9_]+[[:space:]]*=[[:space:]]*errorrender\.Msg[A-Za-z0-9_]+[[:space:]]*$' || true)"
  if [[ -n "$kernel_leftover" ]]; then
    printf 'NON-ALIAS content in %s:\n%s\n' "$kernel_aliases" "$kernel_leftover" >&2
    printf 'Kernel copy must stay alias-only; add wording in platform/errorrender/copy*.go.\n' >&2
    exit 1
  fi
fi

echo "backend copy: OK (${#banned_variants[@]} banned variants absent across ${#tables[@]} tables)"
