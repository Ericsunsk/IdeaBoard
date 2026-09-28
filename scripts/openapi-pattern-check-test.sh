#!/usr/bin/env bash
# openapi-pattern-check-test.sh
#
# Self-test for scripts/openapi-pattern-check.sh. Builds synthetic spec trees
# under a temp REPO_ROOT and asserts the exit code matches expectations.
# This is what the ops contract lane should invoke; without it, a bug in the
# pattern checker would silently pass.
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
checker="$script_dir/openapi-pattern-check.sh"

write_specs() {
  local root="$1"
  local ws_pat="$2"
  local task_pat="$3"
  mkdir -p "$root/apps/api"
  cat > "$root/apps/api/openapi.public.yaml" <<EOF
openapi: 3.0.3
info: {title: t, version: 1.0.0}
paths: {}
components:
  schemas:
    WorkspaceId:
      type: string
      pattern: '$ws_pat'
    TaskId:
      type: string
      pattern: '$task_pat'
EOF
  cat > "$root/apps/api/openapi.control.yaml" <<EOF
openapi: 3.0.3
info: {title: t, version: 1.0.0}
paths: {}
components:
  parameters:
    ControlWorkspaceId:
      name: workspace_id
      in: path
      required: true
      schema:
        type: string
        pattern: '$ws_pat'
EOF
  cat > "$root/apps/api/openapi.events.yaml" <<EOF
openapi: 3.0.3
info: {title: t, version: 1.0.0}
paths: {}
components:
  parameters:
    WorkspaceId:
      name: workspace_id
      in: path
      required: true
      schema:
        type: string
        pattern: '$ws_pat'
EOF
}

# Case 1: the real tree passes.
if REPO_ROOT="$script_dir/.." bash "$checker" >/dev/null 2>&1; then
  echo "OK: clean tree passes"
else
  echo "FAIL: clean tree should pass" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Case 2: a drifted ws_ pattern in one spec must fail.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
python3 - "$tmp/apps/api/openapi.control.yaml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read().replace("^ws_[A-Za-z0-9_-]+$", "^ws_[A-Za-z0-9]+$")
open(p, "w").write(s)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: drifted ws_ pattern should fail" >&2
  exit 1
fi
echo "OK: drifted ws_ pattern fails"

# Case 3: a drifted task_ pattern in one spec must fail.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
python3 - "$tmp/apps/api/openapi.events.yaml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read()
assert "^task_" not in s
open(p, "w").write(s + """components_extra:
  schemas:
    TaskId:
      type: string
      pattern: '^task_[A-Za-z0-9]+$'
""")
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: drifted task_ pattern should fail" >&2
  exit 1
fi
echo "OK: drifted task_ pattern fails"

# Case 4: the allowlisted loose asset_ prefix passes alongside canonical asset_.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
cat > "$tmp/apps/api/openapi.public.yaml" <<'EOF'
openapi: 3.0.3
info: {title: t, version: 1.0.0}
paths: {}
components:
  schemas:
    MediaAssetId:
      type: string
      pattern: '^asset_[A-Za-z0-9_-]+$'
    BatchGetMediaAssetsRequest:
      type: object
      properties:
        asset_ids:
          type: array
          items:
            type: string
            pattern: '^asset_'
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: allowlisted loose asset_ prefix passes"
else
  echo "FAIL: allowlisted loose asset_ prefix should pass" >&2
  exit 1
fi

# Case 5: the same owner name with two distinct patterns must fail.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
cat >> "$tmp/apps/api/openapi.public.yaml" <<'EOF'
    WidgetSku:
      type: string
      pattern: '^[a-z]+$'
EOF
cat >> "$tmp/apps/api/openapi.control.yaml" <<'EOF'
    ControlWidgetSku:
      name: widget_sku
      in: query
      required: false
      schema:
        type: string
        pattern: '^[a-z0-9]+$'
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: shared-name divergence should fail" >&2
  exit 1
fi
echo "OK: shared-name divergence fails"

# Case 6: a brand-new single-occurrence pattern passes (additive).
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
cat >> "$tmp/apps/api/openapi.public.yaml" <<'EOF'
    BrandNewId:
      type: string
      pattern: '^new_[A-Za-z]+$'
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: additive single-occurrence pattern passes"
else
  echo "FAIL: additive single-occurrence pattern should pass" >&2
  exit 1
fi

# Case 7: a missing live spec fails loudly, never skips.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
rm "$tmp/apps/api/openapi.events.yaml"
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: missing spec should fail" >&2
  exit 1
fi
echo "OK: missing spec fails"

# Case 8: an operation without operationId must fail; with one it passes.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
cat >> "$tmp/apps/api/openapi.public.yaml" <<'EOF'
paths:
  /widgets:
    get:
      summary: List widgets
      responses:
        '200':
          description: ok
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: missing operationId should fail" >&2
  exit 1
fi
echo "OK: missing operationId fails"
python3 - "$tmp/apps/api/openapi.public.yaml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read().replace("      summary: List widgets",
                            "      operationId: listWidgets\n      summary: List widgets")
open(p, "w").write(s)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: present operationId passes"
else
  echo "FAIL: present operationId should pass" >&2
  exit 1
fi

# Case 9: a mutation that states neither the IdempotencyKey parameter nor
# an idempotency description must fail; documenting the status passes.
write_specs "$tmp" '^ws_[A-Za-z0-9_-]+$' '^task_[A-Za-z0-9_-]+$'
cat >> "$tmp/apps/api/openapi.public.yaml" <<'EOF'
paths:
  /widgets:
    post:
      operationId: createWidget
      summary: Create a widget
      responses:
        '200':
          description: ok
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "FAIL: undocumented mutation idempotency should fail" >&2
  exit 1
fi
echo "OK: undocumented mutation idempotency fails"
python3 - "$tmp/apps/api/openapi.public.yaml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read().replace("      summary: Create a widget",
                            "      summary: Create a widget\n      description: Requires the Idempotency-Key header.")
open(p, "w").write(s)
EOF
if REPO_ROOT="$tmp" bash "$checker" >/dev/null 2>&1; then
  echo "OK: documented mutation idempotency passes"
else
  echo "FAIL: documented mutation idempotency should pass" >&2
  exit 1
fi

echo "openapi-pattern-check self-test: OK"
