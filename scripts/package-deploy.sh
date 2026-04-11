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
BUNDLE_NAME="feng-bundle-${TAG}-linux-amd64"
BUNDLE_DIR="$RELEASE_ROOT/$BUNDLE_NAME"
ARCHIVE_PATH="$RELEASE_ROOT/${BUNDLE_NAME}.tar.gz"

NPM_CACHE_DIR="$ROOT_DIR/.cache/npm"
GO_CACHE_DIR="$ROOT_DIR/.cache/go-build"
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_DIR="$ROOT_DIR/backend"
FRONTEND_DIST_DIR="$FRONTEND_DIR/dist"
BACKEND_BINARY_TMP="$ROOT_DIR/release-temp/feng-server"
BACKEND_BINARY_DST="$BUNDLE_DIR/backend/feng-server"
BUILD_TIME_UTC="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
COMMIT_SHA="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"

mkdir -p "$NPM_CACHE_DIR" "$GO_CACHE_DIR" "$RELEASE_ROOT" "$ROOT_DIR/release-temp"

echo "[1/5] Installing frontend dependencies"
npm --prefix "$FRONTEND_DIR" ci --cache "$NPM_CACHE_DIR"

echo "[2/5] Building frontend"
npm --prefix "$FRONTEND_DIR" run build

echo "[3/5] Building backend binary (linux/amd64)"
(
  cd "$BACKEND_DIR"
  env GOCACHE="$GO_CACHE_DIR" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o "$BACKEND_BINARY_TMP" ./cmd/main.go
)

echo "[4/5] Assembling release bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p \
  "$BUNDLE_DIR/backend/uploads" \
  "$BUNDLE_DIR/frontend" \
  "$BUNDLE_DIR/deploy/nginx" \
  "$BUNDLE_DIR/deploy/systemd"

cp "$BACKEND_BINARY_TMP" "$BACKEND_BINARY_DST"
cp -R "$FRONTEND_DIST_DIR" "$BUNDLE_DIR/frontend/dist"
cp "$ROOT_DIR/DEPLOYMENT.md" "$BUNDLE_DIR/DEPLOYMENT.md"
cp "$ROOT_DIR/README.md" "$BUNDLE_DIR/README.md"
cp "$ROOT_DIR/development.md" "$BUNDLE_DIR/development.md"
cp "$BACKEND_DIR/.env.example" "$BUNDLE_DIR/backend/.env.example"

cat > "$BUNDLE_DIR/deploy/systemd/feng.service" <<'EOF'
[Unit]
Description=FENG Backend
After=network.target

[Service]
WorkingDirectory=/opt/feng/backend
ExecStart=/opt/feng/backend/feng-server
Restart=always
RestartSec=3
EnvironmentFile=/opt/feng/backend/.env

[Install]
WantedBy=multi-user.target
EOF

cat > "$BUNDLE_DIR/deploy/nginx/feng.conf" <<'EOF'
server {
    listen 80;
    server_name _;

    root /opt/feng/frontend/dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /uploads/ {
        proxy_pass http://127.0.0.1:8080;
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
frontend_dist=frontend/dist
backend_binary=backend/feng-server
EOF

(
  cd "$RELEASE_ROOT"
  tar -czf "$ARCHIVE_PATH" "$BUNDLE_NAME"
  shasum -a 256 "$(basename "$ARCHIVE_PATH")" > SHA256SUMS.txt
)

echo "[5/5] Done"
echo "Archive: $ARCHIVE_PATH"
