#!/usr/bin/env bash

set -euo pipefail

STACK_DIR="${IDEABOARD_STACK_DIR:-/opt/stacks/ideaboard}"
COMPOSE_FILE="${COMPOSE_FILE:-$STACK_DIR/docker/compose.yml}"
ENV_FILE="${ENV_FILE:-$STACK_DIR/docker/.env}"
BIN_PATH="${IDEABOARD_BIN_PATH:-/usr/local/bin/ideaboard}"
YES="${IDEABOARD_YES:-0}"
REMOVE_VOLUMES="${IDEABOARD_REMOVE_VOLUMES:-0}"
KEEP_STACK="${IDEABOARD_KEEP_STACK:-0}"

usage() {
  cat <<EOF
Usage: uninstall-vps.sh --yes [--volumes] [--keep-stack]

Options:
  --yes, -y       Confirm uninstall.
  --volumes       Also remove Docker volumes: mysql-data, redis-data, api-uploads.
  --keep-stack    Keep $STACK_DIR after stopping containers.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes|-y)
      YES=1
      ;;
    --volumes)
      REMOVE_VOLUMES=1
      ;;
    --keep-stack)
      KEEP_STACK=1
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

die() {
  echo "Error: $*" >&2
  exit 1
}

is_enabled() {
  local value="$1"
  [[ "$value" == "1" || "$value" == "true" || "$value" == "yes" ]]
}

guard_stack_dir() {
  case "$STACK_DIR" in
    ""|"/"|"/opt"|"/opt/stacks"|"$HOME")
      die "unsafe stack directory: $STACK_DIR"
      ;;
  esac
}

confirm_uninstall() {
  if is_enabled "$YES"; then
    return
  fi

  if [[ ! -t 0 ]]; then
    die "refusing to uninstall without --yes in non-interactive mode"
  fi

  echo "This will stop and remove the IdeaBoard stack."
  echo "Stack directory: $STACK_DIR"
  if is_enabled "$REMOVE_VOLUMES"; then
    echo "Docker volumes: remove"
  else
    echo "Docker volumes: keep"
  fi
  read -r -p "Type DELETE to continue: " answer
  [[ "$answer" == "DELETE" ]] || die "uninstall canceled"
}

docker_compose_down() {
  if [[ ! -f "$COMPOSE_FILE" ]]; then
    echo "Skip Docker cleanup: missing $COMPOSE_FILE"
    return
  fi

  if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
    echo "Skip Docker cleanup: docker compose is not available"
    return
  fi

  local compose_args=("-f" "$COMPOSE_FILE")
  if [[ -f "$ENV_FILE" ]]; then
    compose_args+=("--env-file" "$ENV_FILE")
  fi

  local down_args=("down" "--remove-orphans")
  if is_enabled "$REMOVE_VOLUMES"; then
    down_args+=("-v")
  fi

  docker compose "${compose_args[@]}" "${down_args[@]}"
}

remove_cli() {
  if [[ ! -L "$BIN_PATH" ]]; then
    return
  fi

  local target
  target="$(readlink "$BIN_PATH" || true)"
  if [[ "$target" == "$STACK_DIR/scripts/deploy-backend-stack.sh" ]]; then
    rm -f "$BIN_PATH"
    echo "Removed $BIN_PATH"
  else
    echo "Skip removing $BIN_PATH: it does not point to this stack"
  fi
}

remove_stack_dir() {
  if is_enabled "$KEEP_STACK"; then
    echo "Kept $STACK_DIR"
    return
  fi

  if [[ -e "$STACK_DIR" ]]; then
    rm -rf "$STACK_DIR"
    echo "Removed $STACK_DIR"
  fi
}

guard_stack_dir
confirm_uninstall
docker_compose_down
remove_cli
remove_stack_dir

if is_enabled "$REMOVE_VOLUMES"; then
  echo "IdeaBoard stack removed with Docker volumes."
else
  echo "IdeaBoard stack removed. Docker volumes were kept."
fi
