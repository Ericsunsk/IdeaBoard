#!/usr/bin/env bash
set -euo pipefail

# This operation intentionally removes clean, tracked source files from a VPS
# checkout. Keep its installed copy outside the checkout when deploying old tags.
if [[ $# -ne 2 || $1 != --apply || $2 != /* ]]; then
  echo 'Usage: deployment-checkout.sh --apply /absolute/deployment-checkout' >&2
  exit 1
fi

cd -- "$2"
repo=$(pwd -P)
if [[ $(git rev-parse --show-toplevel) != "$repo" ]] ||
  [[ $(git rev-parse --is-shallow-repository) != false ]]; then
  echo 'Deployment checkout must be a repository root with complete Git history.' >&2
  exit 1
fi
if git symbolic-ref -q HEAD >/dev/null; then
  echo 'Deployment checkout must have a detached release HEAD.' >&2
  exit 1
fi
if [[ -n $(git status --porcelain --untracked-files=no) ]]; then
  echo 'Refusing to change a deployment checkout with tracked modifications.' >&2
  exit 1
fi

# App source, Dockerfiles and workflow inputs remain available through Git
# objects for manifest validation; running containers supply code and migrations.
paths=(
  .gitignore
  .infisical.yaml
  docker/compose.yml
  docker/compose.npm.yml
  docker/.env.example
  docker/images.env.example
  docker/postgres/init-runtime-roles.sh
  scripts/deploy-stack.sh
  scripts/infisical-run.sh
  scripts/release-artifacts.py
)
for path in "${paths[@]}"; do
  mode=$(git ls-tree HEAD -- "$path")
  case "$mode" in
    '100644 blob '* | '100755 blob '*) ;;
    *)
      echo "Release is missing a regular deployment file: $path" >&2
      exit 1
      ;;
  esac
done

# These tiny operational sources are kept when the selected release has them.
# Their absence on an older tag does not prevent deploying that tag.
for path in scripts/production-deploy-entrypoint.sh scripts/deployment-checkout.sh; do
  mode=$(git ls-tree HEAD -- "$path")
  case "$mode" in
    '100644 blob '* | '100755 blob '*) paths+=("$path") ;;
    '') ;;
    *)
      echo "Release has an invalid deployment operation file: $path" >&2
      exit 1
      ;;
  esac
done

# Non-cone patterns exclude unrelated root files as well as source directories.
# Git preserves untracked and ignored runtime state; never use git clean here.
for path in "${paths[@]}"; do
  printf '/%s\n' "$path"
done | git sparse-checkout set --no-cone --stdin

for path in "${paths[@]}"; do
  if [[ ! -f $path || -L $path ]]; then
    echo "Deployment file was not materialized safely: $path" >&2
    exit 1
  fi
done
echo 'Deployment-only working tree applied; Git history and local runtime state preserved.'
