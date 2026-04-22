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
API_DIR="$ROOT_DIR/apps/api"
WEB_DIST_DIR="$WEB_DIR/dist"
API_BINARY_TMP="$ROOT_DIR/release-temp/ideaboard-server"
API_BINARY_DST="$BUNDLE_DIR/apps/api/ideaboard-server"
BUILD_TIME_UTC="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
COMMIT_SHA="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"

mkdir -p "$NPM_CACHE_DIR" "$GO_CACHE_DIR" "$GO_MOD_CACHE_DIR" "$RELEASE_ROOT" "$ROOT_DIR/release-temp"

echo "[1/5] Installing web dependencies"
npm --prefix "$WEB_DIR" ci --cache "$NPM_CACHE_DIR"

echo "[2/5] Building web app"
npm --prefix "$WEB_DIR" run build

echo "[3/5] Building api binary (linux/amd64)"
(
  cd "$API_DIR"
  env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o "$API_BINARY_TMP" ./cmd
)

echo "[4/5] Assembling release bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p \
  "$BUNDLE_DIR/apps/api/uploads" \
  "$BUNDLE_DIR/apps/web" \
  "$BUNDLE_DIR/deploy/nginx" \
  "$BUNDLE_DIR/deploy/systemd"

cp "$API_BINARY_TMP" "$API_BINARY_DST"
cp -R "$WEB_DIST_DIR" "$BUNDLE_DIR/apps/web/dist"
cp "$ROOT_DIR/DEPLOYMENT.md" "$BUNDLE_DIR/DEPLOYMENT.md"
cp "$ROOT_DIR/README.md" "$BUNDLE_DIR/README.md"
cp "$ROOT_DIR/DEVELOPMENT.md" "$BUNDLE_DIR/DEVELOPMENT.md"
cp "$API_DIR/.env.example" "$BUNDLE_DIR/apps/api/.env.example"

cat > "$BUNDLE_DIR/deploy/systemd/ideaboard.service" <<'EOF'
[Unit]
Description=IdeaBoard API
After=network.target

[Service]
WorkingDirectory=/opt/ideaboard/apps/api
ExecStart=/opt/ideaboard/apps/api/ideaboard-server
Restart=always
RestartSec=3
EnvironmentFile=/opt/ideaboard/apps/api/.env

[Install]
WantedBy=multi-user.target
EOF

cat > "$BUNDLE_DIR/deploy/nginx/ideaboard.conf" <<'EOF'
server {
    listen 80;
    server_name _;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    client_max_body_size 30m;

    root /opt/ideaboard/apps/web/dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:6000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /uploads/ {
        proxy_pass http://127.0.0.1:6000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

cat > "$BUNDLE_DIR/RELEASE_INFO.txt" <<EOF
release=$TAG
commit=$COMMIT_SHA
built_at=$BUILD_TIME_UTC
bundle=$BUNDLE_NAME
web_dist=apps/web/dist
api_binary=apps/api/ideaboard-server
EOF

(
  cd "$RELEASE_ROOT"
  tar -czf "$ARCHIVE_PATH" "$BUNDLE_NAME"
  shasum -a 256 "$(basename "$ARCHIVE_PATH")" > SHA256SUMS.txt
)

echo "[5/5] Done"
echo "Archive: $ARCHIVE_PATH"
