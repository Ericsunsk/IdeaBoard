#!/usr/bin/env bash

set -euo pipefail

STACK_DIR="${IDEABOARD_STACK_DIR:-/opt/stacks/ideaboard}"
REPO_URL="${IDEABOARD_REPO_URL:-https://github.com/Ericsunsk/IdeaBoard.git}"
BRANCH="${IDEABOARD_BRANCH:-main}"
EDITOR_BIN="${EDITOR:-nano}"
BIN_PATH="${IDEABOARD_BIN_PATH:-/usr/local/bin/ideaboard}"
GIT_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

git_with_auth() {
  if [[ -n "$GIT_TOKEN" ]]; then
    git -c "http.https://github.com/.extraheader=AUTHORIZATION: bearer $GIT_TOKEN" "$@"
    return
  fi
  git "$@"
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

mkdir -p "$(dirname "$STACK_DIR")"

if [[ -d "$STACK_DIR/.git" ]]; then
  git_with_auth -C "$STACK_DIR" fetch --prune origin "$BRANCH"
  git_with_auth -C "$STACK_DIR" checkout "$BRANCH"
  git_with_auth -C "$STACK_DIR" pull --ff-only origin "$BRANCH"
else
  rm -rf "$STACK_DIR"
  git_with_auth clone --branch "$BRANCH" "$REPO_URL" "$STACK_DIR"
fi

chmod +x "$STACK_DIR/scripts/deploy-backend-stack.sh"
if [[ -w "$(dirname "$BIN_PATH")" ]]; then
  ln -sf "$STACK_DIR/scripts/deploy-backend-stack.sh" "$BIN_PATH"
else
  echo "Skip installing $BIN_PATH: directory is not writable" >&2
fi
"$STACK_DIR/scripts/deploy-backend-stack.sh" init-env

if [[ -t 0 || -e /dev/tty ]]; then
  "$EDITOR_BIN" "$STACK_DIR/docker/.env.production" </dev/tty >/dev/tty
fi

if [[ -n "$GIT_TOKEN" && -z "${GHCR_TOKEN:-}" ]]; then
  GHCR_TOKEN="$GIT_TOKEN" "$STACK_DIR/scripts/deploy-backend-stack.sh"
else
  "$STACK_DIR/scripts/deploy-backend-stack.sh"
fi
