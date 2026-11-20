#!/usr/bin/env bash
# Usage: loadtest/run.sh <label>   (run from the repo root, stack already up, keys.json exists)
# Resets usage_logs and balances of the load-test users (local compose database only),
# runs k6 in Docker, waits for async usage rows to drain, and prints req/s, p95 and
# how many usage rows were written.
set -euo pipefail
LABEL=${1:?label}
VUS=${VUS:-64}
DURATION=${DURATION:-30s}
BASE=${BASE_URL:-http://127.0.0.1:8080}
mysqlq() { docker compose exec -T mysql sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" gateway -N'; }
mkdir -p loadtest/results
echo "DELETE FROM usage_logs; UPDATE users SET balance=1000000000 WHERE email LIKE 'loadtest-%'; UPDATE api_keys SET rpm_limit=100000000 WHERE name='loadtest';" | mysqlq
docker run --rm --user "$(id -u):$(id -g)" --network host -v "$PWD/loadtest:/loadtest" -e VUS="$VUS" -e DURATION="$DURATION" -e BASE_URL="$BASE" \
  grafana/k6:0.54.0 run --quiet --summary-export=/loadtest/results/"$LABEL".json /loadtest/chat.js \
  | tee loadtest/results/"$LABEL".txt || echo "k6 exited non-zero (threshold failed)"
python3 - "$LABEL" <<'PY'
import json, sys
d = json.load(open(f"loadtest/results/{sys.argv[1]}.json"))["metrics"]
print("REQS", d["http_reqs"]["count"], "RPS %.1f" % d["http_reqs"]["rate"], "P95 %.2f ms" % d["http_req_duration"]["p(95)"],
      "AVG %.2f ms" % d["http_req_duration"]["avg"], "FAILED_RATE", d["http_req_failed"]["value"])
PY
prev=-1
for i in $(seq 1 60); do
  rows=$(echo "SELECT COUNT(*) FROM usage_logs;" | mysqlq)
  [ "$rows" = "$prev" ] && break
  prev=$rows
  sleep 2
done
echo "usage_logs rows after drain: $rows"
