#!/usr/bin/env bash
# Runs chat.js against the compose stack once per gateway configuration, N times each.
# Configurations differ only in environment variables of the gateway container, same image:
#   c0 original behaviour: unbounded MySQL pool, last_used_at on every call, in-process settle, sync usage row
#   c1 c0 + MySQL pool of 64 connections
#   c2 c1 + last_used_at written at most once a minute per key
#   c3 c2 + usage rows through Kafka, batch-inserted by the billing service
#   c4 c3 + settlement over gRPC to the billing service (the default architecture)
# Usage: loadtest/matrix.sh [runs-per-config]   (repo root, stack up, loadtest/keys.json exists)
set -euo pipefail
RUNS=${1:-2}
export WARMUP=${WARMUP:-5s}

apply() { # label, env assignments...
  local label=$1; shift
  env "$@" docker compose up -d --no-deps --force-recreate gateway >/dev/null 2>&1
  for _ in $(seq 1 60); do
    [ "$(docker compose ps gateway --format '{{.Health}}')" = healthy ] && break
    sleep 1
  done
  echo "=== $label: $*"
  DURATION=$WARMUP loadtest/run.sh "warmup" >/dev/null 2>&1 || true
}

declare -A CFG
CFG[c0]="BILLING_ADDR= KAFKA_BROKERS= DB_MAX_OPEN_CONNS=0 DB_MAX_IDLE_CONNS=0 LAST_USED_INTERVAL=0"
CFG[c1]="BILLING_ADDR= KAFKA_BROKERS= DB_MAX_OPEN_CONNS=64 DB_MAX_IDLE_CONNS=64 LAST_USED_INTERVAL=0"
CFG[c2]="BILLING_ADDR= KAFKA_BROKERS= DB_MAX_OPEN_CONNS=64 DB_MAX_IDLE_CONNS=64 LAST_USED_INTERVAL=1m"
CFG[c3]="BILLING_ADDR= DB_MAX_OPEN_CONNS=64 DB_MAX_IDLE_CONNS=64 LAST_USED_INTERVAL=1m"
CFG[c4]="DB_MAX_OPEN_CONNS=64 DB_MAX_IDLE_CONNS=64 LAST_USED_INTERVAL=1m"

for c in ${CONFIGS:-c0 c1 c2 c3 c4}; do
  # shellcheck disable=SC2086
  apply "$c" ${CFG[$c]}
  for r in $(seq 1 "$RUNS"); do
    loadtest/run.sh "$c${TAG:-}-run$r" 2>&1 | grep -E "^REQS|usage_logs rows"
  done
done
