#!/usr/bin/env bash
# Usage: wal-archive.sh <source-wal-path> <wal-filename>
# archive_command helper (DEV-122, E95). Copies a completed WAL segment into
# ARCHIVE_DIR (a directory standing in for the backup bucket). Never overwrites
# an existing segment with different bytes. Designed for archive_timeout = 300
# (RPO 5 minutes). Do not point ARCHIVE_DIR at a production data directory.
set -euo pipefail
src="${1:?source WAL path required}"
name="${2:?WAL file name required}"
dir="${ARCHIVE_DIR:?ARCHIVE_DIR is required}"
case "$dir" in
  ""|"/"|"/var/lib/postgresql"|"/var/lib/postgresql/data") echo "refusing dangerous ARCHIVE_DIR=$dir" >&2; exit 2 ;;
esac
mkdir -p "$dir"
dest="$dir/$name"
if [ -e "$dest" ]; then
  if cmp -s "$src" "$dest"; then
    exit 0
  fi
  echo "refusing to overwrite existing WAL $dest with different bytes" >&2
  exit 2
fi
cp -- "$src" "$dest.partial"
sync "$dest.partial" 2>/dev/null || true
mv -- "$dest.partial" "$dest"
sync "$dir" 2>/dev/null || true
