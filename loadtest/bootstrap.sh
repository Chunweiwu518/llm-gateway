#!/usr/bin/env bash
# Creates N load-test users through the public API (register, login, create key),
# then writes keys.json for chat.js. Balance and rpm_limit are raised by run.sh
# (the default key limit is 60 rpm). Run from the repo root with the stack up.
set -euo pipefail
N=${N:-32}
BASE=${BASE_URL:-http://127.0.0.1:8080}
OUT=loadtest/keys.json
PW="load-test-pass-1"
keys=()
for i in $(seq 1 "$N"); do
  email="loadtest-$i@example.com"
  curl -s -o /dev/null -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$email\",\"password\":\"$PW\"}" || true
  jar=$(mktemp)
  curl -sf -c "$jar" -o /dev/null -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$email\",\"password\":\"$PW\"}"
  key=$(curl -sf -b "$jar" -X POST "$BASE/api/keys" -H 'Content-Type: application/json' -d '{"name":"loadtest"}' \
    | python3 -c 'import sys,json; print(json.load(sys.stdin)["key"])')
  rm -f "$jar"
  keys+=("$key")
done
printf '%s\n' "${keys[@]}" | python3 -c 'import sys,json; print(json.dumps([l.strip() for l in sys.stdin if l.strip()]))' > "$OUT"
echo "wrote $OUT with ${#keys[@]} keys"
