#!/usr/bin/env bash
# Exercise built production images against isolated PostgreSQL/TLS and HTTP/WS fixtures.
# Usage: scripts/production-runtime-test.sh [auth-image] [auth-migrator-image] [realtime-image]
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
auth_image="${1:-ideaboard-auth-bun-final:local}"
migrate_image="${2:-ideaboard-auth-migrate-bun-final:local}"
realtime_image="${3:-ideaboard-realtime-bun-final:local}"
fixture="$(mktemp -d)"
# The runtime user is UID 1000; hosted runners may own the fixture as UID 1001.
chmod 0755 "$fixture"
network="ideaboard-runtime-test-$$"
pg_container="$network-pg"
auth_container="$network-auth"
cleanup() {
  docker rm -f "$auth_container" "$pg_container" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$fixture"
}
trap cleanup EXIT
openssl req -x509 -newkey rsa:2048 -noenc -keyout "$fixture/server.key" \
  -out "$fixture/server.crt" -days 2 -subj '/CN=pg' \
  -addext 'subjectAltName=DNS:pg,DNS:localhost,IP:127.0.0.1' >/dev/null 2>&1
chmod 0644 "$fixture/server.key"
cat >"$fixture/runtime.env" <<'ENV'
NODE_ENV=production
AUTH_INTERNAL_KEY=local-poc-auth-internal-key-123456789
BETTER_AUTH_SECRET=local-poc-better-auth-session-key-123456789
APP_SECRETS_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
PUBLIC_ORIGIN=https://auth-poc.invalid
PG_HOST=pg
PG_PORT=5432
PG_DB=ideaboard
AUTH_PG_USER=ideaboard_auth
AUTH_PG_PASSWORD=local-poc-runtime-password
PG_MIGRATION_USER=migrator
PG_MIGRATION_PASSWORD=local-poc-migrator-password
PG_SSLMODE=verify-full
NODE_EXTRA_CA_CERTS=/certs/server.crt
AUTH_HOST=0.0.0.0
API_INTERNAL_URL=http://127.0.0.1:6000
ENV
chmod 0600 "$fixture/runtime.env"
docker network create "$network" >/dev/null
docker run -d --name "$pg_container" --network "$network" --network-alias pg \
  -e POSTGRES_USER=migrator -e POSTGRES_PASSWORD=local-poc-migrator-password \
  -e POSTGRES_DB=ideaboard -v "$fixture:/certs:ro" postgres:17-alpine sh -c \
  'cp /certs/server.* /tmp/ && chown postgres:postgres /tmp/server.* && chmod 0600 /tmp/server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/server.crt -c ssl_key_file=/tmp/server.key' >/dev/null
ready=false
for _ in {1..60}; do
  if docker exec "$pg_container" pg_isready -U migrator >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
[[ "$ready" == true ]] || { docker logs "$pg_container"; exit 1; }
docker exec -i "$pg_container" psql -v ON_ERROR_STOP=1 -U migrator -d ideaboard \
  <"$repo_root/apps/api/internal/db/migrations/20261113000000_baseline.sql" >"$fixture/baseline.log"
docker exec "$pg_container" psql -v ON_ERROR_STOP=1 -U migrator -d ideaboard \
  -c "CREATE ROLE ideaboard_auth LOGIN PASSWORD 'local-poc-runtime-password';" >/dev/null
for _ in 1 2; do
  docker run --rm --network "$network" --env-file "$fixture/runtime.env" \
    -v "$fixture:/certs:ro" "$migrate_image"
done
for image in "$auth_image" "$migrate_image" "$realtime_image"; do
  docker run --rm --entrypoint sh "$image" -c \
    'test "$(id -u)" = 1000 && test -s /etc/ssl/certs/ca-certificates.crt && test ! -e /sbin/apk && test ! -e /usr/lib/libssl.so.3 && bun -e "if (!process.versions.boringssl) process.exit(1)"'
done
run_pg_probe() {
  docker run --rm --network "$network" --env-file "$fixture/runtime.env" \
    -v "$fixture:/certs:ro" -v "$repo_root/scripts/runtime-contracts:/probe:ro" \
    "$@" "$auth_image" bun /probe/postgres-tls.mjs
}
run_pg_probe
run_pg_probe -e NODE_EXTRA_CA_CERTS= -e EXPECT_REJECT=1
pg_ip="$(docker inspect "$pg_container" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')"
run_pg_probe -e "PG_HOST=$pg_ip" -e EXPECT_REJECT=1
docker run -d --name "$auth_container" --network "$network" --network-alias auth \
  --env-file "$fixture/runtime.env" -v "$fixture:/certs:ro" "$auth_image" >/dev/null
ready=false
for _ in {1..30}; do
  if docker exec "$auth_container" bun -e "fetch('http://127.0.0.1:6001/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))" >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
[[ "$ready" == true ]] || { docker logs "$auth_container"; exit 1; }
docker run --rm --network "$network" --env-file "$fixture/runtime.env" \
  -v "$repo_root/scripts/runtime-contracts:/probe:ro" "$auth_image" bun /probe/auth.mjs
docker run --rm -v "$repo_root/scripts/runtime-contracts:/probe:ro" \
  "$realtime_image" bun /probe/realtime.mjs
printf 'Production runtime network contracts passed\n'
