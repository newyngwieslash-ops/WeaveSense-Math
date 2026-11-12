#!/usr/bin/env bash
# Idempotently create or update a deploy .env so that:
# - MIGRATE_DATABASE_URL uses the PostgreSQL bootstrap user (superuser)
# - DATABASE_URL uses the application role (flow_app, NOSUPERUSER NOBYPASSRLS)
# - MATH_SERVICE_KEY / FLOW_MATH_* are present and the math service is addressed
#   on the compose network
# Usage: sync-edu-deploy-env.sh ENV_FILE
set -Eeuo pipefail

FILE="${1:?env file path required}"

env_get() {
    # last assignment wins, same as the install scripts
    sed -n "s/^${1}=//p" "$FILE" 2>/dev/null | tail -n 1
}

env_set() {
    local key="$1"
    local value="$2"
    local temporary
    temporary="$(mktemp "${FILE}.XXXXXX")"
    if [[ -f "$FILE" ]]; then
        awk -v key="$key" -v value="$value" '
            BEGIN { updated=0 }
            $0 ~ "^" key "=" { print key "=" value; updated=1; next }
            { print }
            END { if (!updated) print key "=" value }
        ' "$FILE" > "$temporary"
        if [[ -e "$FILE" ]]; then
            chmod --reference="$FILE" "$temporary" 2>/dev/null || chmod 600 "$temporary"
        else
            chmod 600 "$temporary"
        fi
    else
        printf '%s=%s\n' "$key" "$value" > "$temporary"
        chmod 600 "$temporary"
    fi
    mv "$temporary" "$FILE"
}

url_encode() {
    # passwords from openssl rand -hex need no encoding; keep a conservative encoder
    python3 -c 'import sys,urllib.parse; sys.stdout.write(urllib.parse.quote(sys.stdin.read(), safe=""))' <<<"$1"
}

rewrite_url_user() {
    # rewrite_url_user URL USER PASSWORD -> URL with user/password replaced
    python3 -c '
import sys, urllib.parse
url, user, password = sys.argv[1], sys.argv[2], sys.argv[3]
parts = urllib.parse.urlsplit(url)
netloc = parts.netloc
host = netloc
if "@" in netloc:
    host = netloc.rsplit("@", 1)[1]
auth = f"{urllib.parse.quote(user, safe="")}:{urllib.parse.quote(password, safe="")}"
print(urllib.parse.urlunsplit((parts.scheme, auth + "@" + host, parts.path, parts.query, parts.fragment)))
' "$1" "$2" "$3"
}

url_user() {
    python3 -c '
import sys, urllib.parse
parts = urllib.parse.urlsplit(sys.argv[1])
if "@" not in parts.netloc:
    raise SystemExit(0)
print(urllib.parse.unquote(parts.netloc.rsplit("@", 1)[0].split(":")[0]))
' "$1"
}

created=0
if [[ ! -f "$FILE" ]]; then
    created=1
    umask 077
    : > "$FILE"
    chmod 600 "$FILE"
fi

postgres_db="$(env_get POSTGRES_DB)"
postgres_user="$(env_get POSTGRES_USER)"
postgres_password="$(env_get POSTGRES_PASSWORD)"
app_user="$(env_get APP_DB_USER)"
app_password="$(env_get APP_DB_PASSWORD)"
database_url="$(env_get DATABASE_URL)"
migrate_url="$(env_get MIGRATE_DATABASE_URL)"

if [[ -z "$postgres_db" ]]; then
    postgres_db="open_ai_canvas"
    env_set POSTGRES_DB "$postgres_db"
fi
if [[ -z "$postgres_user" ]]; then
    postgres_user="open_ai_canvas"
    env_set POSTGRES_USER "$postgres_user"
fi
if [[ -z "$postgres_password" ]]; then
    postgres_password="$(openssl rand -hex 32)"
    env_set POSTGRES_PASSWORD "$postgres_password"
fi
if [[ -z "$app_user" ]]; then
    app_user="flow_app"
    env_set APP_DB_USER "$app_user"
fi
if [[ -z "$app_password" ]]; then
    app_password="$(openssl rand -hex 32)"
    env_set APP_DB_PASSWORD "$app_password"
fi

host_port="postgres:5432"
if [[ -z "$database_url" && -z "$migrate_url" ]]; then
    migrate_url="postgresql://${postgres_user}:${postgres_password}@${host_port}/${postgres_db}?sslmode=disable"
    database_url="postgresql://${app_user}:${app_password}@${host_port}/${postgres_db}?sslmode=disable"
    env_set MIGRATE_DATABASE_URL "$migrate_url"
    env_set DATABASE_URL "$database_url"
else
    if [[ -z "$migrate_url" ]]; then
        # Keep the current URL for migrations (typically the bootstrap superuser).
        migrate_url="$database_url"
        env_set MIGRATE_DATABASE_URL "$migrate_url"
    fi
    current_user="$(url_user "$database_url")"
    if [[ -z "$current_user" || "$current_user" == "$postgres_user" ]]; then
        database_url="$(rewrite_url_user "${database_url:-$migrate_url}" "$app_user" "$app_password")"
        env_set DATABASE_URL "$database_url"
    fi
fi

math_key="$(env_get MATH_SERVICE_KEY)"
if [[ -z "$math_key" ]]; then
    flow_key="$(env_get FLOW_MATH_SERVICE_KEY)"
    if [[ -n "$flow_key" ]]; then
        math_key="$flow_key"
    else
        math_key="$(openssl rand -hex 32)"
    fi
    env_set MATH_SERVICE_KEY "$math_key"
fi
flow_key="$(env_get FLOW_MATH_SERVICE_KEY)"
if [[ -z "$flow_key" || "$flow_key" != "$math_key" ]]; then
    env_set FLOW_MATH_SERVICE_KEY "$math_key"
fi

if [[ -z "$(env_get FLOW_MATH_SERVICE_URL)" ]]; then
    env_set FLOW_MATH_SERVICE_URL "http://math:8790"
fi
if [[ -z "$(env_get MATH_LISTEN)" ]]; then
    env_set MATH_LISTEN "0.0.0.0:8790"
fi
if [[ -z "$(env_get MATH_EVENT_URL)" ]]; then
    env_set MATH_EVENT_URL "http://backend:8080/api/edu-math-events"
fi
if [[ -z "$(env_get EDU_MATH_REQUIRE_SERVICE)" ]]; then
    env_set EDU_MATH_REQUIRE_SERVICE "1"
fi
# FLOW_PRICE_BOOK is optional; leave unset to use the built-in book.
# EDU_* passthroughs stay optional.

if [[ "$created" -eq 1 ]]; then
    echo "wrote new deploy env $FILE"
else
    echo "updated deploy env $FILE"
fi
