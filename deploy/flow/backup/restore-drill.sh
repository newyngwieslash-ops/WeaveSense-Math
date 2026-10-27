#!/usr/bin/env bash
# Usage: restore-drill.sh <source-database-url> <empty-scratch-database-url> [out-dir]
# Restore drill (DEV-122): takes a real backup with backup.sh, restores it with
# restore.sh into an empty scratch database, verifies the manifest, and reports
# the backup point and the measured backup and restore durations. The scratch
# database is never the live one. After a real restore, replay the actions done
# since the backup point before reopening student access:
#   POST /api/edu/restore/replay {"since":"<backup point>"}            (dry run)
#   POST /api/edu/restore/replay {"since":"<backup point>","confirm":true}
set -euo pipefail
src="$1"; dst="$2"; out="${3:-$(mktemp -d)}"
here="$(cd "$(dirname "$0")" && pwd)"
[ "$src" != "$dst" ] || { echo "refusing: scratch database equals source"; exit 2; }
n=$(psql "$dst" -At -c "SELECT count(*) FROM pg_tables WHERE schemaname='public'")
[ "$n" -eq 0 ] || { echo "refusing: scratch database is not empty ($n tables)"; exit 2; }
t0=$(date -u +%s)
point=$(date -u +%Y-%m-%dT%H:%M:%SZ)
dump=$("$here/backup.sh" "$src" "$out")
t1=$(date -u +%s)
verified=$("$here/restore.sh" "$dump" "$dst")
t2=$(date -u +%s)
echo "$verified" | grep -q "restore verified: manifest matches" || { echo "restore drill FAILED: manifest differs"; exit 1; }
tables=$(wc -l < "${dump%.dump}.manifest")
rows=$(awk '{s+=$2} END {print s+0}' "${dump%.dump}.manifest")
echo "restore drill OK: backup_point=$point tables=$tables rows=$rows backup_s=$((t1 - t0)) restore_s=$((t2 - t1)) dump=$(basename "$dump")"
echo "next: POST /api/edu/restore/replay with since=$point (dry run, then confirm), then /api/edu/retention/run, then reopen student access"
