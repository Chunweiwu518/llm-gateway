#!/usr/bin/env bash
# End-to-end check of the async paths on the compose stack (run from the repo root):
# one call through gateway -> billing (gRPC Settle) -> Kafka -> billing consumer -> MySQL, and a
# low-balance alert through RabbitMQ to the notifier. Start billing with a threshold above $1 first:
#   LOW_BALANCE_THRESHOLD=2000000 docker compose up -d billing
set -euo pipefail
BASE=${BASE_URL:-http://127.0.0.1:8080}
EMAIL="events-$(date +%s)@example.com"
PW="events-$(date +%s)-pw"
JAR=$(mktemp)
curl -sf -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" >/dev/null
curl -sf -c "$JAR" -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" >/dev/null
curl -sf -b "$JAR" -X POST "$BASE/api/billing/topup" -H 'Content-Type: application/json' -d '{"amount_usd":1}'; echo
KEY=$(curl -sf -b "$JAR" -X POST "$BASE/api/keys" -H 'Content-Type: application/json' -d '{"name":"events"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["key"])')
echo "--- chat call"
curl -s -w '\nHTTP %{http_code}\n' "$BASE/v1/chat/completions" -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello events"}]}'
echo "--- usage row (written by the billing consumer from Kafka)"
for _ in $(seq 1 30); do
  N=$(curl -sf -b "$JAR" "$BASE/api/usage" | python3 -c 'import sys,json; print(json.load(sys.stdin)["count"])')
  [ "$N" -ge 1 ] && break
  sleep 1
done
echo "usage rows: $N"
curl -sf -b "$JAR" "$BASE/api/billing/balance"; echo
echo "--- low-balance alert in notifier log"
for _ in $(seq 1 30); do
  docker compose logs notifier 2>&1 | grep -q "LOW BALANCE ALERT" && break
  sleep 1
done
docker compose logs notifier 2>&1 | grep "LOW BALANCE ALERT" | tail -1
rm -f "$JAR"
[ "$N" -ge 1 ]
