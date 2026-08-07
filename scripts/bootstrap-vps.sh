#!/usr/bin/env bash

set -euo pipefail

STACK_DIR="${IDEABOARD_STACK_DIR:-/opt/stacks/ideaboard}"
REPO_URL="${IDEABOARD_REPO_URL:-https://github.com/Ericsunsk/IdeaBoard.git}"
BRANCH="${IDEABOARD_BRANCH:-}"
EDITOR_BIN="${EDITOR:-nano}"
BIN_PATH="${IDEABOARD_BIN_PATH:-/usr/local/bin/ideaboard}"
GIT_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
FORCE="${IDEABOARD_FORCE:-0}"
ASKPASS_FILE=""
DEPLOY_SCRIPT="$STACK_DIR/scripts/deploy-stack.sh"

usage() {
  cat <<EOF
Usage: bootstrap-vps.sh --ref <tag-or-commit> [--force]

Options:
  --ref, --branch  Git tag, branch, or commit to deploy. Can also be set with IDEABOARD_BRANCH.
  --force, -f    Move any existing stack directory to a backup, clone again, and restore docker/.env if present.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ref|--branch)
      [[ $# -ge 2 ]] || {
        echo "Missing value for $1" >&2
        usage >&2
        exit 1
      }
      BRANCH="$2"
      shift
      ;;
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

checkout_ref() {
  local dir="$1"

  git_with_auth -C "$dir" fetch --tags --prune origin
  if git -C "$dir" rev-parse --verify --quiet "origin/$BRANCH^{commit}" >/dev/null; then
    git_with_auth -C "$dir" checkout -B "$BRANCH" "origin/$BRANCH"
    return
  fi
  git_with_auth -C "$dir" checkout --detach "$BRANCH"
}

need_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing command: $1" >&2
    exit 1
  }
}

env_file_value() {
  local file="$1"
  local key="$2"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

cosign_verify_disabled() {
  local env_file="$STACK_DIR/docker/.env"
  local value
  [[ -f "$env_file" ]] || return 1
  value="$(env_file_value "$env_file" COSIGN_VERIFY)"
  value="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  [[ "$value" == "false" || "$value" == "0" || "$value" == "no" || "$value" == "off" ]]
}

install_cosign() {
  echo "cosign is missing. Attempting to install it automatically..." >&2

  local os
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  local arch
  arch=$(uname -m)
  if [[ "$arch" == "x86_64" ]]; then
    arch="amd64"
  elif [[ "$arch" == "aarch64" || "$arch" == "arm64" ]]; then
    arch="arm64"
  else
    echo "Warning: unsupported architecture $arch for auto-installing cosign." >&2
    return 1
  fi

  local version="v2.4.1"
  local url="https://github.com/sigstore/cosign/releases/download/${version}/cosign-${os}-${arch}"
  local dest="/usr/local/bin/cosign"

  echo "Downloading cosign from $url..." >&2
  if curl -fsSL "$url" -o "$dest"; then
    chmod +x "$dest"
    echo "cosign has been successfully installed to $dest" >&2
    return 0
  fi

  # Fallback to local project bin if /usr/local/bin is not writable
  local local_bin="$STACK_DIR/bin"
  mkdir -p "$local_bin"
  dest="$local_bin/cosign"
  echo "Retrying download to local bin: $dest..." >&2
  if curl -fsSL "$url" -o "$dest"; then
    chmod +x "$dest"
    export PATH="$local_bin:$PATH"
    echo "cosign has been successfully installed to $dest and added to PATH" >&2
    return 0
  fi

  echo "Error: failed to download cosign automatically." >&2
  return 1
}

need_command git
need_command docker

get_latest_release_tag() {
  local repo_url="$1"
  local tags_output
  tags_output=$(git_with_auth ls-remote --tags --refs "$repo_url" 2>/dev/null || true)
  if [[ -n "$tags_output" ]]; then
    local latest
    latest=$(echo "$tags_output" | awk '{ sub(/^refs\/tags\//, "", $2); print $2 }' | grep '^v[0-9]' | sort -V | tail -n 1)
    if [[ -n "$latest" ]]; then
      echo "$latest"
      return 0
    fi
  fi
  return 1
}

get_remote_default_branch() {
  local repo_url="$1"
  local ref_output
  ref_output=$(git_with_auth ls-remote --symref "$repo_url" HEAD 2>/dev/null || true)
  if [[ -n "$ref_output" ]]; then
    local branch
    branch=$(echo "$ref_output" | awk '/^ref:[[:space:]]*refs\/heads\// { sub(/^ref:[[:space:]]*refs\/heads\//, ""); sub(/[[:space:]]+HEAD$/, ""); print; exit }')
    if [[ -n "$branch" ]]; then
      echo "$branch"
      return 0
    fi
  fi
  echo "main"
}

if [[ -z "$BRANCH" ]]; then
  echo "No --ref or IDEABOARD_BRANCH specified. Detecting latest release tag..."
  if BRANCH=$(get_latest_release_tag "$REPO_URL"); then
    echo "Detected latest release tag: $BRANCH"
  else
    echo "No release tags found. Querying remote default branch..."
    BRANCH=$(get_remote_default_branch "$REPO_URL")
    echo "Using remote default branch: $BRANCH"
  fi
fi

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
  if ! git_with_auth clone "$REPO_URL" "$STACK_DIR" || ! checkout_ref "$STACK_DIR"; then
    mv "$BACKUP_DIR" "$STACK_DIR"
    echo "Clone failed; restored stack from $BACKUP_DIR" >&2
    exit 1
  fi
  restore_env_from_backup "$BACKUP_DIR"
elif [[ -d "$STACK_DIR/.git" ]]; then
  checkout_ref "$STACK_DIR"
else
  if [[ -e "$STACK_DIR" ]]; then
    if [[ -n "$(find "$STACK_DIR" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
      echo "Error: $STACK_DIR exists but is not a git checkout. Move it away or set IDEABOARD_STACK_DIR." >&2
      exit 1
    fi
    rmdir "$STACK_DIR"
  fi
  git_with_auth clone "$REPO_URL" "$STACK_DIR"
  checkout_ref "$STACK_DIR"
fi

chmod +x \
  "$DEPLOY_SCRIPT" \
  "$STACK_DIR/scripts/render-cloudflare-real-ip-conf.sh" \
  "$STACK_DIR/scripts/uninstall-vps.sh"
chmod 755 "$STACK_DIR/docker/postgres/init-runtime-roles.sh"
if [[ -w "$(dirname "$BIN_PATH")" ]]; then
  ln -sf "$DEPLOY_SCRIPT" "$BIN_PATH"
else
  echo "Skip installing $BIN_PATH: directory is not writable" >&2
fi
"$DEPLOY_SCRIPT" init-env

if [[ -t 0 || -e /dev/tty ]]; then
  "$EDITOR_BIN" "$STACK_DIR/docker/.env" </dev/tty >/dev/tty
fi

if ! command -v cosign >/dev/null 2>&1 && ! cosign_verify_disabled; then
  if ! install_cosign; then
    echo "Error: cosign is required for signed-image deployment. Install it manually or set COSIGN_VERIFY=false in docker/.env." >&2
    exit 1
  fi
fi

if [[ -n "$GIT_TOKEN" && -z "${GHCR_TOKEN:-}" ]]; then
  GHCR_TOKEN="$GIT_TOKEN" "$DEPLOY_SCRIPT"
else
  "$DEPLOY_SCRIPT"
fi
