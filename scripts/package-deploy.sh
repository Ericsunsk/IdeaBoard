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
BUNDLE_NAME="ideaboard-bundle-${TAG}-linux-amd64"
BUNDLE_DIR="$RELEASE_ROOT/$BUNDLE_NAME"
ARCHIVE_PATH="$RELEASE_ROOT/${BUNDLE_NAME}.tar.gz"

NPM_CACHE_DIR="$ROOT_DIR/.cache/npm"
GO_CACHE_DIR="$ROOT_DIR/.cache/go-build"
GO_MOD_CACHE_DIR="$ROOT_DIR/.cache/go-mod"
WEB_DIR="$ROOT_DIR/apps/web"
AUTH_DIR="$ROOT_DIR/apps/auth"
API_DIR="$ROOT_DIR/apps/api"
WEB_DIST_DIR="$WEB_DIR/dist"
AUTH_DIST_DIR="$AUTH_DIR/dist"
API_BINARY_TMP="$ROOT_DIR/release-temp/ideaboard-server"
API_BINARY_DST="$BUNDLE_DIR/apps/api/ideaboard-server"
BUILD_TIME_UTC="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
COMMIT_SHA="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"

mkdir -p "$NPM_CACHE_DIR" "$GO_CACHE_DIR" "$GO_MOD_CACHE_DIR" "$RELEASE_ROOT" "$ROOT_DIR/release-temp"

echo "[1/7] Installing web dependencies"
npm --prefix "$WEB_DIR" ci --cache "$NPM_CACHE_DIR"

echo "[2/7] Installing auth dependencies"
npm --prefix "$AUTH_DIR" ci --cache "$NPM_CACHE_DIR"

echo "[3/7] Building web app"
npm --prefix "$WEB_DIR" run build

echo "[4/7] Building auth service"
npm --prefix "$AUTH_DIR" run build

echo "[5/7] Building api binary (linux/amd64)"
(
  cd "$API_DIR"
  env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o "$API_BINARY_TMP" ./cmd
)

echo "[6/7] Assembling release bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p \
  "$BUNDLE_DIR/apps/auth" \
  "$BUNDLE_DIR/apps/api/uploads" \
  "$BUNDLE_DIR/apps/web" \
  "$BUNDLE_DIR/docker" \
  "$BUNDLE_DIR/docs" \
  "$BUNDLE_DIR/deploy/nginx" \
  "$BUNDLE_DIR/deploy/systemd"

cp "$API_BINARY_TMP" "$API_BINARY_DST"
cp -R "$WEB_DIST_DIR" "$BUNDLE_DIR/apps/web/dist"
cp -R "$AUTH_DIST_DIR" "$BUNDLE_DIR/apps/auth/dist"
cp "$AUTH_DIR/package.json" "$BUNDLE_DIR/apps/auth/package.json"
cp "$AUTH_DIR/package-lock.json" "$BUNDLE_DIR/apps/auth/package-lock.json"
cp "$AUTH_DIR/.env.example" "$BUNDLE_DIR/apps/auth/.env.example"
cp "$ROOT_DIR/DEPLOYMENT.md" "$BUNDLE_DIR/DEPLOYMENT.md"
cp "$ROOT_DIR/README.md" "$BUNDLE_DIR/README.md"
cp "$ROOT_DIR/DEVELOPMENT.md" "$BUNDLE_DIR/DEVELOPMENT.md"
cp "$ROOT_DIR/SYSTEM_PROMPT.md" "$BUNDLE_DIR/SYSTEM_PROMPT.md"
cp "$ROOT_DIR/CHANGELOG.md" "$BUNDLE_DIR/CHANGELOG.md"
cp -R "$ROOT_DIR/docs/." "$BUNDLE_DIR/docs/"
cp "$ROOT_DIR/docker/README.md" "$BUNDLE_DIR/docker/README.md"
cp "$API_DIR/.env.example" "$BUNDLE_DIR/apps/api/.env.example"

cat > "$BUNDLE_DIR/deploy/systemd/ideaboard.service" <<'EOF'
[Unit]
Description=IdeaBoard API
After=network.target

[Service]
WorkingDirectory=/opt/ideaboard/apps/api
User=ideaboard
Group=ideaboard
Environment=APP_ENV=production
ExecStart=/opt/ideaboard/apps/api/ideaboard-server
Restart=always
RestartSec=3
EnvironmentFile=/opt/ideaboard/apps/api/.env

[Install]
WantedBy=multi-user.target
EOF

cat > "$BUNDLE_DIR/deploy/systemd/ideaboard-auth.service" <<'EOF'
[Unit]
Description=IdeaBoard Auth Service
After=network.target

[Service]
WorkingDirectory=/opt/ideaboard/apps/auth
User=ideaboard
Group=ideaboard
Environment=NODE_ENV=production
EnvironmentFile=/opt/ideaboard/apps/auth/.env
ExecStart=/usr/bin/env node /opt/ideaboard/apps/auth/dist/index.js
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

bash "$ROOT_DIR/scripts/render-nginx-template.sh" "$BUNDLE_DIR/deploy/nginx/ideaboard.conf"

cat > "$BUNDLE_DIR/RELEASE_INFO.txt" <<EOF
release=$TAG
commit=$COMMIT_SHA
built_at=$BUILD_TIME_UTC
bundle=$BUNDLE_NAME
web_dist=apps/web/dist
api_binary=apps/api/ideaboard-server
auth_service=apps/auth/dist/index.js
EOF

(
  cd "$RELEASE_ROOT"
  tar -czf "$ARCHIVE_PATH" "$BUNDLE_NAME"
  shasum -a 256 "$(basename "$ARCHIVE_PATH")" > SHA256SUMS.txt
)

echo "[7/7] Done"
echo "Archive: $ARCHIVE_PATH"
