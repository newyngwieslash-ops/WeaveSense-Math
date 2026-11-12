#!/usr/bin/env sh
# Create or update the application role used by the Go backend.
# The role is NOSUPERUSER NOBYPASSRLS and is not the table owner, so RLS is not
# silently bypassed. Migrations keep using the PostgreSQL bootstrap user
# (MIGRATE_DATABASE_URL). This script is idempotent.
set -eu

APP_DB_USER="${APP_DB_USER:-flow_app}"
APP_DB_PASSWORD="${APP_DB_PASSWORD:?APP_DB_PASSWORD is required}"
POSTGRES_USER="${POSTGRES_USER:-open_ai_canvas}"

psql -v ON_ERROR_STOP=1 \
  -v app_user="$APP_DB_USER" \
  -v app_password="$APP_DB_PASSWORD" \
  -v owner_user="$POSTGRES_USER" <<'SQL'
SELECT format(
  'ALTER ROLE %I WITH LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE INHERIT',
  :'app_user',
  :'app_password'
)
WHERE EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
UNION ALL
SELECT format(
  'CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE INHERIT',
  :'app_user',
  :'app_password'
)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
\gexec

SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'app_user')
\gexec
GRANT USAGE ON SCHEMA public TO :"app_user";

SELECT format(
  'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE, REFERENCES, TRIGGER ON TABLES TO %I',
  :'owner_user',
  :'app_user'
)
\gexec
SELECT format(
  'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO %I',
  :'owner_user',
  :'app_user'
)
\gexec

GRANT SELECT, INSERT, UPDATE, DELETE, REFERENCES, TRIGGER ON ALL TABLES IN SCHEMA public TO :"app_user";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO :"app_user";
SQL

flags="$(psql -At -v ON_ERROR_STOP=1 -v app_user="$APP_DB_USER" -c "SELECT rolsuper::text || '|' || rolbypassrls::text FROM pg_roles WHERE rolname = :'app_user'")"
[ "$flags" = "f|f" ] || { echo "application role $APP_DB_USER must be NOSUPERUSER NOBYPASSRLS, got $flags" >&2; exit 1; }
echo "application role $APP_DB_USER is NOSUPERUSER NOBYPASSRLS"
