#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_dir="$(mktemp -d)"

mkdir -p "${fixture_dir}/apps/api/internal/db/migrations" "${fixture_dir}/bin"
cat >"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum" <<'EOF'
h1:fixture
20260818000000_initial.sql h1:fakeinitial
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

check() {
  ATLAS_REPOSITORY_ROOT="${fixture_dir}" \
  ATLAS_MIGRATIONS_DIR="${fixture_dir}/apps/api/internal/db/migrations" \
  ATLAS_BINARY="${fixture_dir}/bin/atlas" \
  ATLAS_DEV_URL='postgres://test:test@127.0.0.1:5432/test?sslmode=disable' \
  ATLAS_BASE_SHA="${base_sha}" \
  ATLAS_ALLOW_DESTRUCTIVE="${ATLAS_ALLOW_DESTRUCTIVE:-}" \
  ATLAS_TEST_LOG="${fixture_dir}/atlas.log" \
  "${script_dir}/atlas-migration-check.sh"
}

# 1. Safe incremental migration passes and reaches `migrate validate`.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818010000_incremental.sql" <<'EOF'
ALTER TABLE fixture_initial ADD COLUMN label text;
EOF
printf '%s\n' '20260818010000_incremental.sql h1:fakeincremental' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
check
grep -Fq 'migrate validate' "${fixture_dir}/atlas.log"

# 2. Destructive migration is blocked without the approved bypass.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818020000_destructive.sql" <<'EOF'
DROP TABLE fixture_initial;
EOF
if check >"${fixture_dir}/destructive.out" 2>&1; then
  echo 'destructive migration unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/destructive.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818020000_destructive.sql"

# 3. Approved destructive migration passes only with marker + env.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818020000_destructive.sql" <<'EOF'
-- destructive: approved
DROP TABLE fixture_initial;
EOF
printf '%s\n' '20260818020000_destructive.sql h1:fakedestructive' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
if check >"${fixture_dir}/destructive-unapproved.out" 2>&1; then
  echo 'unapproved destructive migration unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/destructive-unapproved.out"
ATLAS_ALLOW_DESTRUCTIVE=1 check
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818020000_destructive.sql"
sed -i.bak '/20260818020000_destructive.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 4. Column type changes fail with no bypass (even with the marker + env).
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818030000_typechange.sql" <<'EOF'
-- destructive: approved
ALTER TABLE fixture_initial ALTER COLUMN id TYPE bigint USING id::bigint;
EOF
printf '%s\n' '20260818030000_typechange.sql h1:faketype' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
if ATLAS_ALLOW_DESTRUCTIVE=1 check >"${fixture_dir}/typechange.out" 2>&1; then
  echo 'type change unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'column type change is blocked' "${fixture_dir}/typechange.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818030000_typechange.sql"
sed -i.bak '/20260818030000_typechange.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 5. ADD COLUMN ... NOT NULL without DEFAULT fails; with DEFAULT passes.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818040000_notnull.sql" <<'EOF'
ALTER TABLE fixture_initial ADD COLUMN required text NOT NULL;
EOF
printf '%s\n' '20260818040000_notnull.sql h1:fakenotnull' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
if check >"${fixture_dir}/notnull.out" 2>&1; then
  echo 'NOT NULL without DEFAULT unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'NOT NULL without DEFAULT is blocked' "${fixture_dir}/notnull.out"
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818040000_notnull.sql" <<'EOF'
ALTER TABLE fixture_initial ADD COLUMN required text NOT NULL DEFAULT '';
EOF
check
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818040000_notnull.sql"
sed -i.bak '/20260818040000_notnull.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 6. ALTER COLUMN ... SET NOT NULL fails.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818050000_setnotnull.sql" <<'EOF'
ALTER TABLE fixture_initial ALTER COLUMN label SET NOT NULL;
EOF
printf '%s\n' '20260818050000_setnotnull.sql h1:fakesetnotnull' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
if check >"${fixture_dir}/setnotnull.out" 2>&1; then
  echo 'SET NOT NULL unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'SET NOT NULL is blocked' "${fixture_dir}/setnotnull.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818050000_setnotnull.sql"
sed -i.bak '/20260818050000_setnotnull.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 7. Added migration missing from atlas.sum fails (forgot `atlas migrate hash`).
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818060000_nosum.sql" <<'EOF'
ALTER TABLE fixture_initial ADD COLUMN extra text;
EOF
if check >"${fixture_dir}/nosum.out" 2>&1; then
  echo 'missing atlas.sum entry unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'atlas.sum is missing entry' "${fixture_dir}/nosum.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818060000_nosum.sql"

# 8. Editing tracked history fails.
printf '%s\n' '-- edited' >>"${fixture_dir}/apps/api/internal/db/migrations/20260818000000_initial.sql"
if check >"${fixture_dir}/history.out" 2>&1; then
  echo 'edited history unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'history is immutable' "${fixture_dir}/history.out"
git -C "${fixture_dir}" checkout -q -- apps/api/internal/db/migrations/20260818000000_initial.sql

# 9. Stale version (not after base max) fails.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260817000000_stale.sql" <<'EOF'
ALTER TABLE fixture_initial ADD COLUMN stale text;
EOF
printf '%s\n' '20260817000000_stale.sql h1:fakestale' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
if check >"${fixture_dir}/stale.out" 2>&1; then
  echo 'stale version unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'does not sort after base max' "${fixture_dir}/stale.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260817000000_stale.sql"
sed -i.bak '/20260817000000_stale.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 10. Duplicate version prefixes fail.
cp "${fixture_dir}/apps/api/internal/db/migrations/20260818010000_incremental.sql" \
  "${fixture_dir}/apps/api/internal/db/migrations/20260818010000_duplicate.sql"
printf '%s\n' '20260818010000_duplicate.sql h1:fakeduplicate' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
if check >"${fixture_dir}/duplicate.out" 2>&1; then
  echo 'duplicate version unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'duplicate migration version' "${fixture_dir}/duplicate.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818010000_duplicate.sql"
sed -i.bak '/20260818010000_duplicate.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 11. Multiline and multi-action SQL cannot bypass individual risk rules.
risk_path="${fixture_dir}/apps/api/internal/db/migrations/20260818065000_risk.sql"
printf '%s\n' '20260818065000_risk.sql h1:fakerisk' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
expect_risk_blocked() {
  local expected="$1"
  if check >"${fixture_dir}/risk.out" 2>&1; then
    echo "risk migration unexpectedly passed: ${expected}" >&2
    exit 1
  fi
  grep -Fq "${expected}" "${fixture_dir}/risk.out"
}
cat >"${risk_path}" <<'EOF'
ALTER TABLE fixture_initial
  ALTER COLUMN id TYPE bigint;
EOF
expect_risk_blocked 'column type change is blocked'
cat >"${risk_path}" <<'EOF'
alter table fixture_initial
  alter column id set data
  type bigint;
EOF
expect_risk_blocked 'column type change is blocked'
cat >"${risk_path}" <<'EOF'
-- destructive: approved
ALTER TABLE fixture_initial ALTER id SET DATA TYPE bigint;
EOF
ATLAS_ALLOW_DESTRUCTIVE=1 expect_risk_blocked 'column type change is blocked'
cat >"${risk_path}" <<'EOF'
ALTER TABLE fixture_initial
  ALTER COLUMN label SET NOT NULL;
EOF
expect_risk_blocked 'SET NOT NULL is blocked'
cat >"${risk_path}" <<'EOF'
ALTER TABLE fixture_initial
  ADD COLUMN safe integer DEFAULT 0,
  ADD COLUMN unsafe text NOT NULL;
EOF
expect_risk_blocked 'NOT NULL without DEFAULT is blocked'
cat >"${risk_path}" <<'EOF'
ALTER TABLE fixture_initial
  ADD COLUMN unsafe text NOT NULL,
  ADD CONSTRAINT positive CHECK (id > 0);
EOF
expect_risk_blocked 'NOT NULL without DEFAULT is blocked'

# Defaults and constraints belong to their own actions; expression commas,
# quoted text, and comments must not invent statement/action boundaries.
cat >"${risk_path}" <<'EOF'
ALTER TABLE fixture_initial
  ADD COLUMN safe text NOT NULL DEFAULT concat('a, ADD COLUMN fake text NOT NULL;', 'b'),
  ADD CONSTRAINT positive CHECK (id > 0);
ALTER TABLE fixture_initial ADD CONSTRAINT non_null CHECK (label IS NOT NULL);
-- ALTER TABLE fixture_initial ALTER COLUMN id TYPE bigint;
/* ALTER TABLE fixture_initial ALTER COLUMN label SET NOT NULL; */
EOF
check
cat >"${risk_path}" <<'EOF'
ALTER TABLE fixture_initial
  ADD COLUMN safe integer DEFAULT 0,
  ADD COLUMN unsafe text NOT NULL CHECK (length(unsafe) > length('DEFAULT'));
EOF
expect_risk_blocked 'NOT NULL without DEFAULT is blocked'
cat >"${risk_path}" <<'EOF'
alter table fixture_initial
  drop column label;
EOF
expect_risk_blocked 'destructive SQL is blocked'
rm "${risk_path}"
sed -i.bak '/20260818065000_risk.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"

# 12. Staged additions are part of local checks even with an explicit base.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818070000_staged.sql" <<'EOF'
DROP TABLE fixture_initial;
EOF
git -C "${fixture_dir}" add apps/api/internal/db/migrations/20260818070000_staged.sql
if check >"${fixture_dir}/staged.out" 2>&1; then
  echo 'staged destructive migration unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/staged.out"

# 13. Clean-break rebaseline (D old baseline + D old chain + A higher baseline)
# passes the history gate via the narrow exemption.
git -C "${fixture_dir}" reset -q -- apps/api/internal/db/migrations/20260818070000_staged.sql
rm "${fixture_dir}/apps/api/internal/db/migrations/20260818070000_staged.sql"
git -C "${fixture_dir}" add -A apps/api/internal/db/migrations
git -C "${fixture_dir}" commit -qm 'fixture history for clean-break tests'
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818050000_baseline.sql" <<'EOF'
CREATE TABLE cb_old (id integer PRIMARY KEY);
EOF
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260818060000_chain.sql" <<'EOF'
ALTER TABLE cb_old ADD COLUMN label text;
EOF
printf '%s\n' '20260818050000_baseline.sql h1:fakeoldbaseline' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
printf '%s\n' '20260818060000_chain.sql h1:fakechain' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
git -C "${fixture_dir}" add -A apps/api/internal/db/migrations
git -C "${fixture_dir}" commit -qm 'old baseline and chain'
base_sha="$(git -C "${fixture_dir}" rev-parse HEAD)"
git -C "${fixture_dir}" rm -q apps/api/internal/db/migrations/20260818050000_baseline.sql \
  apps/api/internal/db/migrations/20260818060000_chain.sql
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819000000_baseline.sql" <<'EOF'
CREATE TABLE cb_new (id integer PRIMARY KEY);
EOF
git -C "${fixture_dir}" add apps/api/internal/db/migrations/20260819000000_baseline.sql
sed -i.bak '/20260818050000_baseline.sql/d; /20260818060000_chain.sql/d' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"
printf '%s\n' '20260819000000_baseline.sql h1:fakenewbaseline' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
check >"${fixture_dir}/cleanbreak.out" 2>&1
grep -Fq 'clean-break rebaseline detected' "${fixture_dir}/cleanbreak.out"

# 14. The exemption is narrow: destructive DDL in the new baseline is still
# blocked without marker + env, even during a clean break.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819000000_baseline.sql" <<'EOF'
DROP TABLE cb_new;
EOF
if check >"${fixture_dir}/cleanbreak-destructive.out" 2>&1; then
  echo 'destructive clean-break baseline unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/cleanbreak-destructive.out"
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819000000_baseline.sql" <<'EOF'
-- destructive: approved
DROP TABLE cb_new;
EOF
if check >"${fixture_dir}/cleanbreak-unapproved.out" 2>&1; then
  echo 'unapproved destructive clean-break baseline unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/cleanbreak-unapproved.out"
ATLAS_ALLOW_DESTRUCTIVE=1 check >"${fixture_dir}/cleanbreak-approved.out" 2>&1
grep -Fq 'clean-break rebaseline detected' "${fixture_dir}/cleanbreak-approved.out"
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819000000_baseline.sql" <<'EOF'
CREATE TABLE cb_new (id integer PRIMARY KEY);
EOF

# 15. A second added baseline voids the exemption.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819010000_baseline.sql" <<'EOF'
CREATE TABLE cb_extra (id integer PRIMARY KEY);
EOF
if check >"${fixture_dir}/cleanbreak-second.out" 2>&1; then
  echo 'second added baseline unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'history is immutable' "${fixture_dir}/cleanbreak-second.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260819010000_baseline.sql"

# 16. An added baseline that does not sort after the deleted baseline voids
# the exemption.
git -C "${fixture_dir}" rm -q --cached apps/api/internal/db/migrations/20260819000000_baseline.sql
mv "${fixture_dir}/apps/api/internal/db/migrations/20260819000000_baseline.sql" \
  "${fixture_dir}/apps/api/internal/db/migrations/20260818040000_baseline.sql"
sed -i.bak 's/20260819000000_baseline.sql/20260818040000_baseline.sql/' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"
if check >"${fixture_dir}/cleanbreak-lower.out" 2>&1; then
  echo 'lower-version baseline unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'history is immutable' "${fixture_dir}/cleanbreak-lower.out"
mv "${fixture_dir}/apps/api/internal/db/migrations/20260818040000_baseline.sql" \
  "${fixture_dir}/apps/api/internal/db/migrations/20260819000000_baseline.sql"
sed -i.bak 's/20260818040000_baseline.sql/20260819000000_baseline.sql/' "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
rm "${fixture_dir}/apps/api/internal/db/migrations/atlas.sum.bak"
git -C "${fixture_dir}" add apps/api/internal/db/migrations/20260819000000_baseline.sql

# 17. A modified tracked migration alongside the rebaseline voids the exemption.
printf '%s\n' '-- edited' >>"${fixture_dir}/apps/api/internal/db/migrations/20260818000000_initial.sql"
if check >"${fixture_dir}/cleanbreak-modified.out" 2>&1; then
  echo 'modified history alongside clean break unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'history is immutable' "${fixture_dir}/cleanbreak-modified.out"
git -C "${fixture_dir}" checkout -q -- apps/api/internal/db/migrations/20260818000000_initial.sql

# 18. An added non-baseline migration alongside the rebaseline voids the exemption.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819020000_extra.sql" <<'EOF'
ALTER TABLE cb_new ADD COLUMN extra text;
EOF
if check >"${fixture_dir}/cleanbreak-extra.out" 2>&1; then
  echo 'non-baseline addition alongside clean break unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'history is immutable' "${fixture_dir}/cleanbreak-extra.out"
rm "${fixture_dir}/apps/api/internal/db/migrations/20260819020000_extra.sql"

# 19. Rename blind spot: a staged rename reports status R (not A), so a
# --diff-filter=A scan would miss the new path entirely. The destructive
# gate must still block a renamed migration without marker + env.
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819030000_rename_me.sql" <<'EOF'
-- rename-me fixture: keep enough stable bytes so git detects the rename.
-- rename-me fixture: keep enough stable bytes so git detects the rename.
ALTER TABLE cb_new ADD COLUMN rename_me text;
EOF
printf '%s\n' '20260819030000_rename_me.sql h1:fakerenameme' >>"${fixture_dir}/apps/api/internal/db/migrations/atlas.sum"
git -C "${fixture_dir}" add -A apps/api/internal/db/migrations
git -C "${fixture_dir}" commit -qm 'rename-me fixture'
git -C "${fixture_dir}" mv apps/api/internal/db/migrations/20260819030000_rename_me.sql \
  apps/api/internal/db/migrations/20260819040000_renamed.sql
cat >"${fixture_dir}/apps/api/internal/db/migrations/20260819040000_renamed.sql" <<'EOF'
-- rename-me fixture: keep enough stable bytes so git detects the rename.
-- rename-me fixture: keep enough stable bytes so git detects the rename.
ALTER TABLE cb_new ADD COLUMN rename_me text;
DROP TABLE cb_new;
EOF
git -C "${fixture_dir}" add apps/api/internal/db/migrations/20260819040000_renamed.sql
git -C "${fixture_dir}" diff --name-status HEAD -- apps/api/internal/db/migrations/ | grep -q '^R' || {
  echo 'fixture did not produce a rename status' >&2
  exit 1
}
if check >"${fixture_dir}/rename-destructive.out" 2>&1; then
  echo 'renamed destructive migration unexpectedly passed' >&2
  exit 1
fi
grep -Fq 'destructive SQL is blocked' "${fixture_dir}/rename-destructive.out"

echo 'Atlas migration check tests passed'
