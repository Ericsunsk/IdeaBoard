#!/usr/bin/env bash
# Exercise the actual workflow shell guards and both review gates over a failed
# change followed by an unrelated commit. No network, database or Atlas needed.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "${script_dir}/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "${fixture}"' EXIT
export GIT_AUTHOR_NAME=Fixture GIT_AUTHOR_EMAIL=fixture@example.invalid GIT_COMMITTER_NAME=Fixture GIT_COMMITTER_EMAIL=fixture@example.invalid
REAL_GO="$(command -v go)"
export REAL_GO
export OASDIFF_FIXTURE="${fixture}/oasdiff" OASDIFF_TEST_LOG="${fixture}/oasdiff.log"
mkdir -p "${fixture}/repo/apps/api/internal/db/migrations" "${fixture}/repo/scripts/lib" "${fixture}/bin"
cp "${script_dir}/openapi-breaking-check.sh" "${fixture}/repo/scripts/"
cp "${script_dir}/lib/common.sh" "${fixture}/repo/scripts/lib/"

# Extract only the literal run block of these known steps. The workflow
# contract checker separately validates their typed input bindings.
workflow_run() {
  awk -v step="$1" '
    $0 == "      - name: " step { found = 1; next }
    found && /^        run: \|$/ { body = 1; next }
    body && /^          / { sub(/^          /, ""); print; next }
    body { exit }
  ' "${root}/.github/workflows/api.yml"
}
workflow_run 'Verify trusted migration review range' > "${fixture}/migration-range.sh"
workflow_run 'Check for breaking OpenAPI changes' > "${fixture}/openapi-range.sh"
[[ -s "${fixture}/migration-range.sh" && -s "${fixture}/openapi-range.sh" ]]

cat > "${fixture}/bin/atlas" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
cat > "${fixture}/bin/go" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == install && "$2" == github.com/oasdiff/oasdiff@* ]]; then
  cp "${OASDIFF_FIXTURE}" "${GOBIN}/oasdiff"
  chmod +x "${GOBIN}/oasdiff"
else
  exec "${REAL_GO}" "$@"
fi
MOCK
cat > "${fixture}/oasdiff" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == breaking && "$2" == --fail-on && "$3" == ERR ]]
printf '%s\n' compared >> "${OASDIFF_TEST_LOG}"
# The fixture marks a removed contract. The real checker must invoke oasdiff
# for the cumulative range; event.before would see no contract change at all.
if grep -q '^# BREAKING' "$5" && ! grep -q '^# BREAKING' "$4"; then exit 1; fi
MOCK
chmod +x "${fixture}/bin/atlas" "${fixture}/bin/go"
export PATH="${fixture}/bin:${PATH}"

cd "${fixture}/repo"
git init -q
printf 'CREATE TABLE fixture_initial (id integer PRIMARY KEY);\n' > apps/api/internal/db/migrations/20260101000000_initial.sql
printf 'h1:fixture\n20260101000000_initial.sql h1:initial\n' > apps/api/internal/db/migrations/atlas.sum
for name in public control events; do
  printf 'openapi: 3.0.3\ninfo: {title: Fixture, version: "1"}\npaths: {}\n' > "apps/api/openapi.${name}.yaml"
done
git add .
git commit -qm 'successful verified baseline'
verified="$(git rev-parse HEAD)"
printf 'DROP TABLE fixture_initial;\n' > apps/api/internal/db/migrations/20260102000000_destructive.sql
printf '20260102000000_destructive.sql h1:destructive\n' >> apps/api/internal/db/migrations/atlas.sum
printf '# BREAKING removed operation\n' >> apps/api/openapi.public.yaml
git add .
git commit -qm 'failed migration and breaking contract'
failed="$(git rev-parse HEAD)"
printf 'Unrelated documentation.\n' > README.md
git add README.md
git commit -qm 'later unrelated docs'
head="$(git rev-parse HEAD)"

migration_review() {
  ATLAS_BASE_SHA="$1" ATLAS_HEAD_SHA="${2:-${head}}" bash -e -o pipefail "${fixture}/migration-range.sh" || return $?
  ATLAS_REPOSITORY_ROOT="${fixture}/repo" \
    ATLAS_MIGRATIONS_DIR="${fixture}/repo/apps/api/internal/db/migrations" \
    ATLAS_BASE_SHA="$1" ATLAS_HEAD_SHA="${2:-${head}}" \
    ATLAS_BINARY="${fixture}/bin/atlas" ATLAS_DEV_URL='postgres://fixture.invalid/test' \
    bash "${script_dir}/atlas-migration-check.sh"
}
openapi_review() {
  BASE_SHA="$1" HEAD_SHA="${2:-${head}}" OPENAPI_BREAKING_LABELS='' bash -e -o pipefail "${fixture}/openapi-range.sh"
}
reject() {
  local expected="$1"; shift
  if "$@" > "${fixture}/failure.log" 2>&1; then
    echo "Cumulative review unexpectedly passed: ${expected}" >&2
    exit 1
  fi
  grep -Fq "${expected}" "${fixture}/failure.log"
}

# The old event.before policy goes green despite the earlier failed change.
migration_review "${failed}" > "${fixture}/old-migration.log"
openapi_review "${failed}" > "${fixture}/old-openapi.log"
[[ ! -f "${OASDIFF_TEST_LOG}" ]]
# The trusted range must actually run and reject both existing gates.
reject 'destructive SQL is blocked' migration_review "${verified}"
reject 'BREAKING changes detected' openapi_review "${verified}"
[[ "$(wc -l < "${OASDIFF_TEST_LOG}" | tr -d ' ')" == 1 ]]

# Unknown history must never become a successful no-op, including full runs.
reject 'verified ancestor base SHA is required' migration_review ''
reject 'verified ancestor base SHA is required' openapi_review ''
reject 'head does not match checkout' migration_review "${verified}" "${verified}"
reject 'head does not match checkout' openapi_review "${verified}" "${verified}"
foreign="$(printf 'unrelated\n' | git commit-tree 'HEAD^{tree}')"
reject 'base is not a checkout ancestor' migration_review "${foreign}"
reject 'base is not a checkout ancestor' openapi_review "${foreign}"
# A full release may compare a commit already certified by CI against itself.
migration_review "${head}" > "${fixture}/certified-migration.log"
openapi_review "${head}" > "${fixture}/certified-openapi.log"
echo 'CI cumulative migration/OpenAPI review fixtures passed'
