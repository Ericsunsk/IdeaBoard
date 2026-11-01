#!/usr/bin/env bash
# infisical-run.sh
#
# Thin wrapper that injects Infisical secrets into one command via
# `infisical run` (process-env injection, no files). This is the ONLY
# supported Infisical injection shape for the business stack:
#
#   scripts/infisical-run.sh --env prod --path / --path /ai-proxy -- scripts/deploy-stack.sh deploy --manifest release-manifest.json
#   scripts/infisical-run.sh --env dev --path / --path /ai-proxy -- scripts/deploy-stack.sh config
#
# Namespace: single project (`ideaboard`, --projectId or .infisical.yaml),
# three envs (dev/staging/prod, REQUIRED --env), secrets live at root path
# `/` (repeatable --path, default `/`), with ONE exception: the ai-proxy
# Worker bindings live in folder `/ai-proxy` (dev/prod 同形；api/api-worker
# consumers pass `--path / --path /ai-proxy` for the union; the root copy of
# WORKER_INTERNAL_SECRET is pending manual dashboard deletion, see runbook §2).
# Full mapping: docs/runbooks/secrets-infisical.md.
#
# Two tiers, never merged (see runbook):
#   primary  = this script (Infisical live read)
#   fallback = dotenvx run -f docker/.env (offline, frozen copy)
# This wrapper REFUSES to wrap a dotenvx command so the two tiers can never
# run stacked in one process (no double-source merges).
#
# Auth (Infisical Cloud):
#   local dev : `infisical login` (human user; CLI defaults to Cloud, no
#               --domain needed; SSO enforced in the Cloud dashboard)
#   CI/VPS    : read-only service token via INFISICAL_TOKEN env (never a flag,
#               never committed); the token is already project+env scoped.
#
# Cloud site URL: defaults to https://app.infisical.com. Override only via
# --domain / INFISICAL_DOMAIN — e.g.
# for the documented return path to self-hosted (see runbook §8).

set -euo pipefail

# Infisical Cloud default. Keep as the single constant so a future
# self-hosted return only touches --domain, never the injection shape.
CLOUD_SITE_URL="https://app.infisical.com"

PROJECT_ID="${INFISICAL_PROJECT_ID:-}"
DOMAIN="${INFISICAL_DOMAIN:-}"
ENV_NAME=""
PATHS=()

usage() {
  cat >&2 <<'EOF'
usage: scripts/infisical-run.sh --env <dev|staging|prod> [--path / ...] [--project-id <id>] [--domain <url>] -- <command...>
example: scripts/infisical-run.sh --env prod --path / --path /ai-proxy -- scripts/deploy-stack.sh deploy --manifest release-manifest.json
EOF
  exit 2
}

while (($# > 0)); do
  case "$1" in
    --env)
      [[ $# -ge 2 ]] || usage
      ENV_NAME="$2"
      shift 2
      ;;
    --env=*)
      ENV_NAME="${1#--env=}"
      shift
      ;;
    --path)
      [[ $# -ge 2 ]] || usage
      PATHS+=("$2")
      shift 2
      ;;
    --path=*)
      PATHS+=("${1#--path=}")
      shift
      ;;
    --project-id)
      [[ $# -ge 2 ]] || usage
      PROJECT_ID="$2"
      shift 2
      ;;
    --project-id=*)
      PROJECT_ID="${1#--project-id=}"
      shift
      ;;
    --domain)
      [[ $# -ge 2 ]] || usage
      DOMAIN="$2"
      shift 2
      ;;
    --domain=*)
      DOMAIN="${1#--domain=}"
      shift
      ;;
    --)
      shift
      break
      ;;
    -h | --help) usage ;;
    *) usage ;;
  esac
done

[[ -n "$ENV_NAME" ]] || usage
[[ $# -gt 0 ]] || usage
case "$ENV_NAME" in
  dev | staging | prod) ;;
  *) echo "error: --env must be one of dev|staging|prod (got '$ENV_NAME')" >&2 && exit 2 ;;
esac
if ((${#PATHS[@]} == 0)); then
  PATHS=("/")
fi

# Never stack the two secret tiers in one process.
for arg in "$@"; do
  if [[ "$arg" == *"dotenvx"* ]]; then
    echo "error: refusing to run dotenvx inside infisical-run (two tiers must never merge; pick one: secrets-infisical.md)" >&2
    exit 1
  fi
done

command -v infisical >/dev/null 2>&1 || {
  echo "error: infisical CLI not found (install: npm i -g @infisical/cli; then: infisical login)" >&2
  exit 1
}

args=(run "--env=$ENV_NAME")
for p in "${PATHS[@]}"; do
  args+=(--path "$p")
done
if [[ -n "$PROJECT_ID" ]]; then
  args+=(--projectId "$PROJECT_ID")
fi
if [[ -n "$DOMAIN" ]]; then
  args+=(--domain "$DOMAIN")
else
  # Cloud-first: pin the Cloud site URL explicitly so `--domain` in a
  # process listing always shows where secrets were read from.
  args+=(--domain "$CLOUD_SITE_URL")
fi
# Owner credentials never come from Cloud; deployment reads its local source.
args+=(-- env -u POSTGRES_PASSWORD -u PG_MIGRATION_USER -u PG_MIGRATION_PASSWORD "$@")

export IDEABOARD_SECRETS_SOURCE=infisical
exec infisical "${args[@]}"
