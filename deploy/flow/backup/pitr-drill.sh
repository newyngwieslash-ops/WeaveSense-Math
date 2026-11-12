#!/usr/bin/env bash
# Usage: pitr-drill.sh [work-dir]
# Local / CI WAL + point-in-time recovery drill (DEV-122, E95).
# Starts a throwaway PostgreSQL cluster with archive_mode=on, writes a known
# row, takes a base backup, writes a later row, and restores to a time between
# the two writes. The later row must be absent; the earlier row must be present.
# Never touches an existing data directory or a live database URL.
set -euo pipefail
# initdb and postgres refuse to run as root. Official images stay root when the
# entrypoint is overridden (as in CI). Re-exec as the postgres package user.
if [ "$(id -u)" = "0" ]; then
  if command -v su-exec >/dev/null 2>&1; then
    exec su-exec postgres "$0" "$@"
  elif command -v gosu >/dev/null 2>&1; then
    exec gosu postgres "$0" "$@"
  else
    echo "pitr-drill cannot run as root; re-run as the postgres user (or install su-exec/gosu)" >&2
    exit 2
  fi
fi
here="$(cd "$(dirname "$0")" && pwd)"
work="${1:-$(mktemp -d)}"
mkdir -p "$work"
need() { command -v "$1" >/dev/null || { echo "pitr-drill requires $1" >&2; exit 2; }; }
need initdb
need postgres
need psql
need pg_basebackup
need pg_isready

data="$work/pgdata"
arch="$work/wal"
base="$work/base"
restored="$work/restored"
mkdir -p "$arch" "$base"

initdb -D "$data" --auth-local=trust --auth-host=trust --no-sync -U postgres >/dev/null
# listen_addresses is empty (unix sockets only). Use a fixed port so the socket
# name is .s.PGSQL.$port; port=0 would be .s.PGSQL.0 and pg_isready (default 5432) would miss it.
port=55432
export PGHOST="$work" PGPORT="$port" PGUSER=postgres
cat >> "$data/postgresql.conf" <<CONF
port = $port
unix_socket_directories = '$work'
listen_addresses = ''
wal_level = replica
archive_mode = on
archive_timeout = 1
archive_command = 'ARCHIVE_DIR=$arch $here/wal-archive.sh %p %f'
fsync = off
synchronous_commit = off
logging_collector = off
CONF

src_log="$work/src-postgres.log"
rst_log="$work/rst-postgres.log"
postgres -D "$data" >"$src_log" 2>&1 &
src_pid=$!
cleanup() {
  if [ -n "${src_pid:-}" ] && kill -0 "$src_pid" 2>/dev/null; then
    kill "$src_pid" 2>/dev/null || true
    wait "$src_pid" 2>/dev/null || true
  fi
  if [ -n "${rst_pid:-}" ] && kill -0 "$rst_pid" 2>/dev/null; then
    kill "$rst_pid" 2>/dev/null || true
    wait "$rst_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

ready() {
  local dir="$1" log="$2" label="$3"
  for i in $(seq 1 80); do
    pg_isready -h "$dir" -p "$port" -U postgres >/dev/null 2>&1 && return 0
    sleep 0.1
  done
  echo "pitr-drill: $label postgres failed to become ready" >&2
  tail -80 "$log" >&2 || true
  return 1
}
ready "$work" "$src_log" source

sock="host=$work port=$port user=postgres dbname=postgres"
psql "$sock" -qc "CREATE TABLE edu_pitr_probe(id int primary key, note text, at timestamptz default now())"
psql "$sock" -qc "INSERT INTO edu_pitr_probe VALUES (1, 'before-backup')"
# Force a WAL switch so the first row is archived.
psql "$sock" -qc "SELECT pg_switch_wal()"
sleep 1

pg_basebackup -h "$work" -p "$port" -U postgres -D "$base" -X none --no-sync -c fast

# Recovery target: just after the backup, before the second write.
target=$(psql "$sock" -At -c "SELECT to_char(clock_timestamp() at time zone 'UTC', 'YYYY-MM-DD HH24:MI:SS')")
sleep 2
psql "$sock" -qc "INSERT INTO edu_pitr_probe VALUES (2, 'after-backup-must-not-survive')"
psql "$sock" -qc "SELECT pg_switch_wal()"
sleep 1
# Stop the source so we don't hold locks; source data dir stays untouched after this.
kill "$src_pid"
wait "$src_pid" 2>/dev/null || true
src_pid=""

"$here/pitr-restore.sh" "$base" "$arch" "$restored" "$target"
# Use a different socket dir so the restored postmaster does not collide.
mkdir -p "$work/restored-run"
echo "unix_socket_directories = '$work/restored-run'" >> "$restored/postgresql.conf"
echo "port = $port" >> "$restored/postgresql.conf"
echo "listen_addresses = ''" >> "$restored/postgresql.conf"
postgres -D "$restored" >"$rst_log" 2>&1 &
rst_pid=$!
ready "$work/restored-run" "$rst_log" restored
rsock="host=$work/restored-run port=$port user=postgres dbname=postgres"
notes=$(psql "$rsock" -At -c "SELECT string_agg(note, ',' order by id) FROM edu_pitr_probe")
count=$(psql "$rsock" -At -c "SELECT count(*) FROM edu_pitr_probe")
if [ "$notes" != "before-backup" ] || [ "$count" != "1" ]; then
  echo "pitr drill FAILED: expected only before-backup, got count=$count notes=$notes" >&2
  echo "---- restored log ----" >&2
  tail -80 "$rst_log" >&2 || true
  echo "---- source log ----" >&2
  tail -40 "$src_log" >&2 || true
  exit 1
fi
echo "pitr drill OK: recovery_target_time=$target rows=1 kept=before-backup dropped=after-backup-must-not-survive work=$work"
