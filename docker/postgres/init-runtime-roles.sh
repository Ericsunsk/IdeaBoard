#!/bin/sh
set -eu

: "${API_PG_PASSWORD:?API_PG_PASSWORD is required}"
: "${AUTH_PG_PASSWORD:?AUTH_PG_PASSWORD is required}"

api_db_user=ideaboard_api
auth_db_user=ideaboard_auth

psql --set ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --set api_user="$api_db_user" \
  --set api_password="$API_PG_PASSWORD" \
  --set auth_user="$auth_db_user" \
  --set auth_password="$AUTH_PG_PASSWORD" <<'SQL'
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

ALTER SCHEMA public OWNER TO ideaboard;
SQL
