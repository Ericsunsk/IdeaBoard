#!/usr/bin/env bash
# check-backend-copy-test.sh
#
# Self-test for scripts/check-backend-copy.sh. Asserts the real tree
# passes, synthetic fixtures with reintroduced banned variants fail
# (negative probe), and canonical-only fixtures pass — including the
# prefix-collision cases where the banned form is a strict substring
# of the canonical wording.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/check-backend-copy.sh"

# Case 1: the real tree passes.
if REPO_ROOT="$script_dir/.." bash "$checker" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

write_table() {
  local root="$1"
  local literal="$2"
  mkdir -p "$root/apps/api/internal/fixture"
  cat > "$root/apps/api/internal/fixture/message.go" <<EOF
package fixture

// Backend-owned user-facing copy for this package.
const (
	msgFixture = "$literal" // Fixture @ fixture.go
)
EOF
}

# Case 2: each reintroduced banned variant must fail.
for variant in '视觉搜索暂时不可用' '请求参数错误' '分页游标非法' \
  '操作频繁，请稍后重试' '认证服务不可用' 'Stripe 账户不可用' \
  '当前工作区钱包受限，暂不可' '读取模型映射失败' \
  '幂等请求与原创建参数不一致' '状态值无效，仅支持0或1'; do
  write_table "$tmp" "$variant"
  if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
    echo "FAIL: reintroduced variant $variant should fail" >&2
    exit 1
  fi
  echo "OK: reintroduced variant fails ($variant)"
done

# Case 3: canonical-only fixtures pass — especially the
# prefix-collision canonicals that contain a banned substring.
for canonical in '视觉搜索暂不可用' '参数错误' '分页游标无效' \
  '操作过于频繁，请稍后重试' '认证服务暂不可用' '当前 Stripe 账户不可用' \
  '当前工作区钱包受限，暂不可用' '读取网关模型映射失败' \
  '幂等请求与原请求不一致' 'status 仅支持 0 或 1'; do
  write_table "$tmp" "$canonical"
  if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
    echo "OK: canonical passes ($canonical)"
  else
    echo "FAIL: canonical should pass ($canonical)" >&2
    exit 1
  fi
done

# Case 4: the runtimehttp kernel table must stay alias-only
# (ADR-0017 follow-up). An alias-only errors.go passes; a table with a
# reintroduced string literal fails even when the wording is canonical.
alias_file="$tmp/apps/api/internal/runtime/interfaces/http/errors.go"
mkdir -p "$(dirname "$alias_file")"
cat > "$alias_file" <<'EOF'
package http

import "ideaboard/api/internal/platform/errorrender"

const (
	MsgBadRequest = errorrender.MsgBadRequest
)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: alias-only kernel table passes"
else
  echo "FAIL: alias-only kernel table should pass" >&2
  exit 1
fi
cat > "$alias_file" <<'EOF'
package http

const (
	MsgBadRequest = "请求体读取失败"
)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: literal kernel table should fail" >&2
  exit 1
fi
echo "OK: literal kernel table fails"

# Case 5: copy*.go tables are scanned too (covers copy_runtime.go).
# A banned variant hiding in copy_runtime.go must fail.
mkdir -p "$tmp/apps/api/internal/platform/errorrender"
cat > "$tmp/apps/api/internal/platform/errorrender/copy_runtime.go" <<'EOF'
package errorrender

const (
	MsgFixtureRuntime = "搜索服务暂时不可用"
)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: banned variant in copy_runtime.go should fail" >&2
  exit 1
fi
echo "OK: banned variant in copy_runtime.go fails"
cat > "$tmp/apps/api/internal/platform/errorrender/copy_runtime.go" <<'EOF'
package errorrender

const (
	MsgFixtureRuntime = "搜索服务暂不可用"
)
EOF
rm -f "$tmp/apps/api/internal/fixture/message.go"
# Case 4 leaves the literal kernel table behind; restore alias-only so
# this case isolates copy_runtime.go coverage.
cat > "$alias_file" <<'EOF'
package http

import "ideaboard/api/internal/platform/errorrender"

const (
	MsgBadRequest = errorrender.MsgBadRequest
)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: canonical-only copy_runtime.go passes"
else
  echo "FAIL: canonical-only copy_runtime.go should pass" >&2
  exit 1
fi

echo "check-backend-copy self-test: OK"
