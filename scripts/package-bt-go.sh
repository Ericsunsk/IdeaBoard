#!/usr/bin/env bash

set -euo pipefail
export LC_ALL=C
export LANG=C

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="$ROOT_DIR/VERSION"

if [[ -n "${1:-}" ]]; then
  TAG="$1"
elif [[ -f "$VERSION_FILE" ]]; then
  TAG="v$(tr -d '[:space:]' < "$VERSION_FILE")"
else
  echo "Missing version. Pass a tag like v0.1.0 or create VERSION." >&2
  exit 1
fi

if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Warning: non-SemVer tag '$TAG' (recommended: vMAJOR.MINOR.PATCH)." >&2
fi

RELEASE_ROOT="$ROOT_DIR/release/$TAG"
BUNDLE_NAME="feng-bt-go-${TAG}-linux-amd64"
BUNDLE_DIR="$RELEASE_ROOT/$BUNDLE_NAME"
APP_DIR="$BUNDLE_DIR/app"
ARCHIVE_PATH="$RELEASE_ROOT/${BUNDLE_NAME}.tar.gz"

NPM_CACHE_DIR="$ROOT_DIR/.cache/npm"
GO_CACHE_DIR="$ROOT_DIR/.cache/go-build"
GO_MOD_CACHE_DIR="$ROOT_DIR/.cache/go-mod"
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTEND_DIST_DIR="$FRONTEND_DIR/dist"
BACKEND_BUILD_DIR="$ROOT_DIR/release-temp/backend-build"
BACKEND_BINARY_TMP="$ROOT_DIR/release-temp/feng-server"

mkdir -p "$NPM_CACHE_DIR" "$GO_CACHE_DIR" "$GO_MOD_CACHE_DIR" "$RELEASE_ROOT" "$ROOT_DIR/release-temp"

echo "[1/5] Installing frontend dependencies"
npm --prefix "$FRONTEND_DIR" ci --cache "$NPM_CACHE_DIR"

echo "[2/5] Building frontend dist"
npm --prefix "$FRONTEND_DIR" run build

echo "[3/5] Preparing backend embed workspace"
rm -rf "$BACKEND_BUILD_DIR"
cp -R "$BACKEND_DIR" "$BACKEND_BUILD_DIR"
rm -rf "$BACKEND_BUILD_DIR/web/dist"
mkdir -p "$BACKEND_BUILD_DIR/web/dist"
cp -R "$FRONTEND_DIST_DIR"/. "$BACKEND_BUILD_DIR/web/dist/"

echo "[4/5] Building backend binary (linux/amd64)"
(
  cd "$BACKEND_BUILD_DIR"
  env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o "$BACKEND_BINARY_TMP" ./cmd/main.go
)

echo "[5/5] Assembling Baota Go bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p "$APP_DIR"

cp "$BACKEND_BINARY_TMP" "$APP_DIR/feng-server"
chmod +x "$APP_DIR/feng-server"
cp "$BACKEND_DIR/.env.example" "$BUNDLE_DIR/.env"

(
  cd "$RELEASE_ROOT"
  tar -czf "$ARCHIVE_PATH" "$BUNDLE_NAME"
  shasum -a 256 "$(basename "$ARCHIVE_PATH")" > SHA256SUMS.txt
)

echo "Done"
echo "Bundle directory: $BUNDLE_DIR"
echo "Archive: $ARCHIVE_PATH"
echo "Contains:"
echo "  - app/feng-server"
echo "  - .env"
