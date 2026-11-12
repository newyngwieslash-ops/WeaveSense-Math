#!/usr/bin/env bash
# Education load-test harness (DEV-119, E95).
# --scaffold-only validates the k6 and vegeta files and exits 0 without
# generating traffic. A formal capacity report still needs U16.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
mode="${1:-}"
if [ "$mode" = "--scaffold-only" ] || [ "$mode" = "--dry-run" ]; then
  test -s "$here/k6-edu.js"
  grep -q 'p(95)<800' "$here/k6-edu.js"
  grep -q 'p(95)<400' "$here/k6-edu.js"
  grep -q 'job_accept' "$here/k6-edu.js"
  grep -q 'auth_rw' "$here/k6-edu.js"
  grep -q '/api/edu/problem-revisions/' "$here/k6-edu.js"
  grep -q '/api/edu/me' "$here/vegeta-targets.txt"
  grep -q '/api/edu/observability' "$here/vegeta-targets.txt"
  echo "edu loadtest scaffold OK: k6 job_accept p95<800ms, auth_rw p95<400ms; vegeta auth-read targets present"
  exit 0
fi
BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
if command -v k6 >/dev/null; then
  BASE_URL="$BASE_URL" k6 run "$here/k6-edu.js"
  exit $?
fi
if command -v vegeta >/dev/null; then
  sed "s#BASE#${BASE_URL}#" "$here/vegeta-targets.txt" | vegeta attack -duration="${DURATION:-10s}" -rate="${AUTH_RATE:-5}" | vegeta report
  exit $?
fi
echo "neither k6 nor vegeta is installed; ran no traffic. Re-run with --scaffold-only to validate files, or install k6/vegeta." >&2
exit 2
