#!/usr/bin/env bash
# Exercise Worker policy through actual startup and rendered Compose adapters.
# Only public examples, derived fixture values and temporary files are read.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=scripts/lib/env-schema.sh
source "$SCRIPT_DIR/lib/env-schema.sh"
fixture_dir="$(mktemp -d)"
trap 'rm -rf -- "$fixture_dir"' EXIT
schema="$REPO_ROOT/apps/ai-proxy/src/contracts.ts"
fail() { printf 'worker-bindings-test: %s\n' "$1" >&2; exit 1; }
worker_binding_check_startup "$REPO_ROOT/process-compose.yml"

# Env is authoritative: a new Worker credential participates without editing a
# separate key registry, and unrelated payload fields never become bindings.
sed '/^export interface Env {$/a\
  WORKER_FIXTURE_BINDING: string;\
  CF_FIXTURE_BINDING: string;
' "$schema" >"$fixture_dir/contracts.ts"
contains_key WORKER_FIXTURE_BINDING "$(worker_binding_keys "$fixture_dir/contracts.ts" api-worker)" || fail 'new Worker credential omitted'
contains_key CF_FIXTURE_BINDING "$(worker_binding_keys "$fixture_dir/contracts.ts" api)" || fail 'new discovery binding omitted'
if contains_key CF_FIXTURE_BINDING "$(worker_binding_keys "$fixture_dir/contracts.ts" api-worker)"; then fail 'discovery leaked to API-worker'; fi
[[ -z "$(worker_binding_keys "$schema" auth)" ]] || fail 'unrelated consumer receives bindings'
[[ "$(worker_binding_paths auth)" == / ]] || fail 'unrelated consumer leaves root'
[[ "$(worker_binding_paths worker)" == "$(worker_binding_source_path)" ]] || fail 'Worker source differs'

# The generic injection adapter keeps its root-only default; explicit policy
# paths still reach the CLI without changing unrelated consumers.
mkdir -p "$fixture_dir/bin"
cat >"$fixture_dir/bin/infisical" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
: >"$FAKE_PATHS"
while (($# > 0)); do
  case "$1" in
    --path) printf '%s\n' "$2" >>"$FAKE_PATHS"; shift 2 ;;
    *) shift ;;
  esac
done
FAKE
chmod +x "$fixture_dir/bin/infisical"
FAKE_PATHS="$fixture_dir/paths" PATH="$fixture_dir/bin:$PATH" bash "$SCRIPT_DIR/infisical-run.sh" --env dev -- true
[[ "$(cat "$fixture_dir/paths")" == "$(worker_binding_paths auth)" ]] || fail 'generic wrapper changed root default'
path_args=()
while IFS= read -r path; do path_args+=(--path "$path"); done < <(worker_binding_paths api)
FAKE_PATHS="$fixture_dir/paths" PATH="$fixture_dir/bin:$PATH" bash "$SCRIPT_DIR/infisical-run.sh" --env dev "${path_args[@]}" -- true
[[ "$(cat "$fixture_dir/paths")" == "$(worker_binding_paths api)" ]] || fail 'generic wrapper lost explicit policy paths'

# Static startup declarations are checked against the same source policy.
sed 's/--path \/ --path \/ai-proxy/--path \//' "$REPO_ROOT/process-compose.yml" >"$fixture_dir/startup.yml"
if worker_binding_check_startup "$fixture_dir/startup.yml" 2>/dev/null; then fail 'API missing folder accepted'; fi
sed 's/--env dev -- npm run dev/--env dev --path \/ai-proxy -- npm run dev/' "$REPO_ROOT/process-compose.yml" >"$fixture_dir/startup.yml"
if worker_binding_check_startup "$fixture_dir/startup.yml" 2>/dev/null; then fail 'unrelated folder consumer accepted'; fi

# The wrapper also accepts --path=VALUE and shell-quoted values. Static
# startup declarations use unquoted literal paths; every alternate form must
# fail closed rather than accidentally being treated as a root-only default.
for path_flags in '--path=/ai-proxy' "--path '/ai-proxy'" '--path "/ai-proxy"' "--path /\${WORKER_FOLDER}"; do
  sed "s|--env dev -- npm run dev|--env dev ${path_flags} -- npm run dev|" "$REPO_ROOT/process-compose.yml" >"$fixture_dir/startup.yml"
  if worker_binding_check_startup "$fixture_dir/startup.yml" 2>/dev/null; then fail 'unparsed startup path accepted'; fi
done

# Render real Compose with public examples and generated fixtures; no container
# starts, daemon access, ignored .env file or Cloud call is needed.
binding_args=()
while IFS= read -r key; do
  [[ -n "$key" ]] || continue
  binding_args+=("$key=fixture-$key")
done < <(worker_binding_keys "$schema" worker)
env "${binding_args[@]}" docker compose -f "$REPO_ROOT/docker/compose.yml" \
  --env-file "$REPO_ROOT/docker/.env.example" --env-file "$REPO_ROOT/docker/images.env.example" \
  config --format json >"$fixture_dir/compose.json"
worker_binding_check_compose "$schema" <"$fixture_dir/compose.json" || fail 'actual Compose projection differs'
while IFS= read -r consumer; do
  while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    jq -e --arg consumer "$consumer" --arg key "$key" \
      '.services[$consumer].environment[$key] == ("fixture-" + $key)' "$fixture_dir/compose.json" >/dev/null || fail 'rendered binding lost its value'
  done < <(worker_binding_keys "$schema" "$consumer")
done <<'CONSUMERS'
api
api-worker
CONSUMERS

# Both missing bindings and an accidental extra recipient must fail the gate.
for mutation in \
  'del(.services.api.environment.CF_AIG_TOKEN)' \
  '.services["api-worker"].environment.CF_AIG_TOKEN="fixture-token"' \
  '.services.auth.environment.WORKER_INTERNAL_SECRET="fixture-secret"'; do
  jq "$mutation" "$fixture_dir/compose.json" >"$fixture_dir/broken.json"
  if worker_binding_check_compose "$schema" <"$fixture_dir/broken.json"; then fail 'invalid Compose projection accepted'; fi
done
printf 'Worker binding source, schema, startup and rendered Compose contracts passed\n'
