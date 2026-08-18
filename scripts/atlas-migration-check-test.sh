#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_dir="$(mktemp -d)"

mkdir -p "${fixture_dir}/apps/api/internal/db/migrations" "${fixture_dir}/bin"
cat >"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum" <<'EOF'
h1:fixture
EOF
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818000000_initial.sql" <<'EOF'
CREATE TABLE fixture_initial (id integer PRIMARY KEY);
EOF
git -C "${fixture_dir}" init -q
git -C "${fixture_dir}" config user.email test@example.com
git -C "${fixture_dir}" config user.name test
git -C "${fixture_dir}" add .
git -C "${fixture_dir}" commit -qm baseline
base_sha="$(git -C "${fixture_dir}" rev-parse HEAD)"

cat >"${fixture_dir}/bin/atlas" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${ATLAS_TEST_LOG}"
EOF
chmod +x "${fixture_dir}/bin/atlas"

cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818010000_incremental.sql" <<'EOF'
ALTER TABLE fixture_initial ADD COLUMN label text;
EOF
ATLAS_REPOSITORY_ROOT="${fixture_dir}" \
ATLAS_MIGRATIONS_DIR="${fixture_dir}/apps/api/internal/db/migrations" \
ATLAS_BINARY="${fixture_dir}/bin/atlas" \
ATLAS_DEV_URL='postgres://test:test@127.0.0.1:5432/test?sslmode=disable' \
ATLAS_BASE_SHA="${base_sha}" \
ATLAS_TEST_LOG="${fixture_dir}/atlas.log" \
"${script_dir}/atlas-migration-check.sh"
grep -Fq 'migrate validate' "${fixture_dir}/atlas.log"

cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818020000_destructive.sql" <<'EOF'
DROP TABLE fixture_initial;
EOF
if ATLAS_REPOSITORY_ROOT="${fixture_dir}" \
  ATLAS_MIGRATIONS_DIR="${fixture_dir}/apps/api/internal/db/migrations" \
  ATLAS_BINARY="${fixture_dir}/bin/atlas" \
  ATLAS_DEV_URL='postgres://test:test@127.0.0.1:5432/test?sslmode=disable' \
  ATLAS_BASE_SHA="${base_sha}" \
  ATLAS_TEST_LOG="${fixture_dir}/atlas.log" \
  "${script_dir}/atlas-migration-check.sh" >"${fixture_dir}/destructive.out" 2>&1; then
  echo 'destructive migration unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/destructive.out"

echo 'Atlas migration check tests passed'
