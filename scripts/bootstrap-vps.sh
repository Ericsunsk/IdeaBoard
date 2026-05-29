#!/usr/bin/env bash

set -euo pipefail

STACK_DIR="${IDEABOARD_STACK_DIR:-/opt/stacks/ideaboard}"
REPO_URL="${IDEABOARD_REPO_URL:-https://github.com/Ericsunsk/IdeaBoard.git}"
BRANCH="${IDEABOARD_BRANCH:-main}"
EDITOR_BIN="${EDITOR:-nano}"
BIN_PATH="${IDEABOARD_BIN_PATH:-/usr/local/bin/ideaboard}"
GIT_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
FORCE="${IDEABOARD_FORCE:-0}"
ASKPASS_FILE=""

usage() {
  cat <<EOF
Usage: bootstrap-vps.sh [--force]

Options:
  --force, -f    Move any existing stack directory to a backup, clone again, and restore docker/.env if present.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --force|-f)
      FORCE=1
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

cleanup() {
  if [[ -n "$ASKPASS_FILE" && -f "$ASKPASS_FILE" ]]; then
    rm -f "$ASKPASS_FILE"
  fi
}
trap cleanup EXIT

git_with_auth() {
  if [[ -n "$GIT_TOKEN" ]]; then
    GIT_ASKPASS="$ASKPASS_FILE" GIT_TERMINAL_PROMPT=0 git "$@"
    return
  fi
  git "$@"
}

is_force_enabled() {
  [[ "$FORCE" == "1" || "$FORCE" == "true" || "$FORCE" == "yes" ]]
}

make_backup_dir() {
  local base candidate index
  base="${STACK_DIR}.backup.$(date +%Y%m%d%H%M%S)"
  candidate="$base"
  index=1
  while [[ -e "$candidate" ]]; do
    candidate="${base}.${index}"
    index=$((index + 1))
  done
  printf '%s\n' "$candidate"
}

restore_env_from_backup() {
  local backup_dir="$1"
  local old_env="$backup_dir/docker/.env"
  local new_env="$STACK_DIR/docker/.env"

  if [[ -f "$old_env" && ! -f "$new_env" ]]; then
    mkdir -p "$(dirname "$new_env")"
    cp "$old_env" "$new_env"
    echo "Restored docker/.env from $backup_dir"
  fi
}

need_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing command: $1" >&2
    exit 1
  }
}

need_command git
need_command docker

if ! docker compose version >/dev/null 2>&1; then
  echo "Missing command: docker compose" >&2
  exit 1
fi

if [[ -n "$GIT_TOKEN" ]]; then
  export GITHUB_TOKEN="$GIT_TOKEN"
  ASKPASS_FILE="$(mktemp)"
  chmod 700 "$ASKPASS_FILE"
  cat > "$ASKPASS_FILE" <<'EOF'
#!/usr/bin/env sh
case "$1" in
  *Username*) printf '%s\n' x-access-token ;;
  *Password*) printf '%s\n' "$GITHUB_TOKEN" ;;
  *) printf '\n' ;;
esac
EOF
fi

mkdir -p "$(dirname "$STACK_DIR")"

if [[ -e "$STACK_DIR" ]] && is_force_enabled; then
  BACKUP_DIR="$(make_backup_dir)"
  mv "$STACK_DIR" "$BACKUP_DIR"
  echo "Backed up existing stack to $BACKUP_DIR"
  if ! git_with_auth clone --branch "$BRANCH" "$REPO_URL" "$STACK_DIR"; then
    mv "$BACKUP_DIR" "$STACK_DIR"
    echo "Clone failed; restored stack from $BACKUP_DIR" >&2
    exit 1
  fi
  restore_env_from_backup "$BACKUP_DIR"
elif [[ -d "$STACK_DIR/.git" ]]; then
  git_with_auth -C "$STACK_DIR" fetch --prune origin "$BRANCH"
  git_with_auth -C "$STACK_DIR" checkout "$BRANCH"
  git_with_auth -C "$STACK_DIR" pull --ff-only origin "$BRANCH"
else
  if [[ -e "$STACK_DIR" ]]; then
    if [[ -n "$(find "$STACK_DIR" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
      echo "Error: $STACK_DIR exists but is not a git checkout. Move it away or set IDEABOARD_STACK_DIR." >&2
      exit 1
    fi
    rmdir "$STACK_DIR"
  fi
  git_with_auth clone --branch "$BRANCH" "$REPO_URL" "$STACK_DIR"
fi

chmod +x "$STACK_DIR/scripts/deploy-backend-stack.sh" "$STACK_DIR/scripts/uninstall-vps.sh"
if [[ -w "$(dirname "$BIN_PATH")" ]]; then
  ln -sf "$STACK_DIR/scripts/deploy-backend-stack.sh" "$BIN_PATH"
else
  echo "Skip installing $BIN_PATH: directory is not writable" >&2
fi
"$STACK_DIR/scripts/deploy-backend-stack.sh" init-env

if [[ -t 0 || -e /dev/tty ]]; then
  "$EDITOR_BIN" "$STACK_DIR/docker/.env" </dev/tty >/dev/tty
fi

if [[ -n "$GIT_TOKEN" && -z "${GHCR_TOKEN:-}" ]]; then
  GHCR_TOKEN="$GIT_TOKEN" "$STACK_DIR/scripts/deploy-backend-stack.sh"
else
  "$STACK_DIR/scripts/deploy-backend-stack.sh"
fi
