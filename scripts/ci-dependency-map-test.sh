#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
(cd "$root/apps/api" && go build -o "$fixture/checker" ../../scripts/ci-dependency-map-check.go)
repo="$fixture/repo"
mkdir -p "$repo/.github/workflows" "$repo/.github/actions/setup-go" "$repo/.github/actions/setup-node" "$repo/scripts" "$repo/apps/api" "$repo/apps/web"
git init -q "$repo"
cat > "$repo/.github/workflows/ci.yml" <<'YAML'
jobs:
  api:
    uses: ./.github/workflows/api.yml
  web:
    uses: ./.github/workflows/web.yml
YAML
cat > "$repo/.github/workflows/api.yml" <<'YAML'
jobs:
  verify:
    steps:
      - uses: ./.github/actions/setup-go
YAML
cat > "$repo/.github/workflows/web.yml" <<'YAML'
jobs:
  verify:
    steps:
      - uses: ./.github/actions/setup-node
YAML
cat > "$repo/.github/actions/setup-go/action.yml" <<'YAML'
runs:
  steps:
    - uses: actions/setup-go@fixed
      with:
        go-version-file: apps/api/go.mod
YAML
printf 'runs: {steps: []}\n' > "$repo/.github/actions/setup-node/action.yml"
printf 'module example.invalid/fixture\n' > "$repo/apps/api/go.mod"
cat > "$repo/scripts/ci-change-classifier.sh" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
read -r input
case "$input" in
  .github/actions/setup-go/*|apps/api/go.mod) printf 'api=true\nweb=false\n' ;;
  .github/actions/setup-node/*|apps/web/*) printf 'api=false\nweb=true\n' ;;
  *) printf 'api=false\nweb=false\n' ;;
esac
BASH
git -C "$repo" add .
"$fixture/checker" --root "$repo" --json "$fixture/map.json" >/dev/null
# Repository/context roots also locate subprocesses or scratch workspaces.
# Their presence alone must not fan out every tracked application file.
printf 'const root = "../../.."; const scratch = "apps/api/"\n' > "$repo/apps/web/paths.ts"
git -C "$repo" add .
"$fixture/checker" --root "$repo" >/dev/null
# An actual new setup action consumer must update the classifier.
printf '      - uses: ./.github/actions/setup-go\n' >> "$repo/.github/workflows/web.yml"
if "$fixture/checker" --root "$repo" > "$fixture/failure" 2>&1; then echo 'missing action consumer accepted' >&2; exit 1; fi
grep -Fq '.github/actions/setup-go/action.yml requires web' "$fixture/failure"
sed '$d' "$repo/.github/workflows/web.yml" > "$fixture/clean.yml"
cp "$fixture/clean.yml" "$repo/.github/workflows/web.yml"
# New cross-app source consumption creates a verifiable dependency edge.
printf 'export const value = 1\n' > "$repo/apps/web/contract.ts"
cat > "$repo/apps/api/contract_test.go" <<'GO'
package fixture
import "path/filepath"
var input = filepath.Join("apps", "web", "contract.ts")
GO
git -C "$repo" add .
if "$fixture/checker" --root "$repo" > "$fixture/failure" 2>&1; then echo 'missing source consumer accepted' >&2; exit 1; fi
grep -Fq 'apps/web/contract.ts requires api' "$fixture/failure"
sed 's@.github/actions/setup-node/\*|apps/web/\*) printf '\''api=false@.github/actions/setup-node/*|apps/web/*) printf '\''api=true@' "$repo/scripts/ci-change-classifier.sh" > "$fixture/classifier"
cp "$fixture/classifier" "$repo/scripts/ci-change-classifier.sh"
"$fixture/checker" --root "$repo" --json "$fixture/map.json" >/dev/null
jq -e '.edges | any(.input == "apps/web/contract.ts" and .consumer == "api")' "$fixture/map.json" >/dev/null
# A precise fixture directory still consumes its tracked descendants.
mkdir -p "$repo/apps/web/fixtures"
printf 'fixture\n' > "$repo/apps/web/fixtures/payload.json"
printf 'package fixture\nvar fixturePath = "apps/web/fixtures"\n' > "$repo/apps/api/directory_test.go"
git -C "$repo" add .
"$fixture/checker" --root "$repo" --json "$fixture/map.json" >/dev/null
jq -e '.edges | any(.input == "apps/web/fixtures/payload.json" and .consumer == "api")' "$fixture/map.json" >/dev/null
# Shared shell tooling in reusable workflow run blocks is also a real consumer.
printf '#!/usr/bin/env bash\n' > "$repo/apps/api/install-atlas.sh"
printf '      - run: ./apps/api/install-atlas.sh /tmp/atlas\n' >> "$repo/.github/workflows/web.yml"
git -C "$repo" add .
if "$fixture/checker" --root "$repo" > "$fixture/failure" 2>&1; then echo 'missing shared shell consumer accepted' >&2; exit 1; fi
grep -Fq 'apps/api/install-atlas.sh requires web' "$fixture/failure"
printf 'CI dependency discovery mutation fixtures passed\n'
