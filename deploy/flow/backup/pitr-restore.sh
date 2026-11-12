#!/usr/bin/env bash
# Usage: pitr-restore.sh <base-backup-dir> <wal-archive-dir> <empty-target-data-dir> <recovery-target-time-UTC>
# Restores a pg_basebackup plus WAL to a point in time (DEV-122, E95).
# The target data directory must be empty and must not be the live data directory
# or the archive directory. After recovery, the instance is a new timeline.
set -euo pipefail
base="${1:?base backup dir required}"
wal="${2:?WAL archive dir required}"
dest="${3:?empty target data dir required}"
when="${4:?recovery target time required, UTC e.g. 2026-11-12 12:00:00}"
here="$(cd "$(dirname "$0")" && pwd)"
for p in "$base" "$wal"; do
  [ -d "$p" ] || { echo "missing directory $p" >&2; exit 2; }
done
case "$dest" in
  ""|"/"|"$base"|"$wal") echo "refusing dangerous target $dest" >&2; exit 2 ;;
esac
if [ -e "$dest" ] && [ -n "$(ls -A "$dest" 2>/dev/null || true)" ]; then
  echo "refusing: target data directory is not empty" >&2
  exit 2
fi
mkdir -p "$dest"
# pg_basebackup -Ft format unpacks as base.tar + pg_wal.tar (or a directory copy).
if [ -f "$base/base.tar" ]; then
  tar -C "$dest" -xf "$base/base.tar"
  if [ -f "$base/pg_wal.tar" ]; then
    mkdir -p "$dest/pg_wal"
    tar -C "$dest/pg_wal" -xf "$base/pg_wal.tar"
  fi
else
  cp -a "$base/." "$dest/"
fi
rm -f "$dest/postmaster.pid"
touch "$dest/recovery.signal"
cat > "$dest/postgresql.auto.conf" <<AUTO
restore_command = 'cp "${wal}/%f" "%p"'
recovery_target_time = '${when}'
recovery_target_action = 'promote'
AUTO
echo "pitr-restore prepared: dest=$dest recovery_target_time=$when"
echo "start this cluster with postgres -D $dest (or pg_ctl), then run POST /api/edu/restore/replay"
echo "hint: $here/restore-drill.sh covers logical restore; this path is WAL/PITR only"
