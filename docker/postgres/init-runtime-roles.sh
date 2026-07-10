#!/bin/sh
set -eu

: "${API_DB_USER:?API_DB_USER is required}"
: "${API_DB_PASSWORD:?API_DB_PASSWORD is required}"
: "${AUTH_DB_USER:?AUTH_DB_USER is required}"
: "${AUTH_DB_PASSWORD:?AUTH_DB_PASSWORD is required}"

psql --set ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --set api_user="$API_DB_USER" \
  --set api_password="$API_DB_PASSWORD" \
  --set auth_user="$AUTH_DB_USER" \
  --set auth_password="$AUTH_DB_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'api_user', :'api_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'api_user')
\gexec

SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'api_user', :'api_password')
\gexec

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'auth_user', :'auth_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'auth_user')
\gexec

SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'auth_user', :'auth_password')
\gexec
SQL
