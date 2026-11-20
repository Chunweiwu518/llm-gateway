#!/usr/bin/env bash
# Kubernetes smoke test: builds the four images, starts a throwaway kind cluster, installs the Helm chart,
# sends one /v1/chat/completions through web -> gateway -> billing (gRPC) -> Kafka -> MySQL, checks the
# usage row and the low-balance alert, then deletes the cluster. Needs docker, kind, kubectl, helm, curl, python3.
# Secrets are random per run and never written to disk.
set -euo pipefail
cd "$(dirname "$0")/../.."

CLUSTER=llm-gateway-smoke
NS=llm-gateway
TAG=0.1.0
PORT=${PORT:-18080}
export KUBECONFIG="$(mktemp)"   # separate kubeconfig: never touches the default one

cleanup() {
  rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "=== smoke failed (exit $rc): pods and recent logs"
    kubectl -n "$NS" get pods -o wide || true
    for d in gateway billing notifier kafka; do
      echo "--- logs $d"; kubectl -n "$NS" logs "deploy/$d" --tail=40 || true
    done
  fi
  [ -n "${PF_PID:-}" ] && kill "$PF_PID" 2>/dev/null || true
  kind delete cluster --name "$CLUSTER" || true
  rm -f "$KUBECONFIG"
}
trap cleanup EXIT

docker build -q -f backend/Dockerfile -t llm-gateway/gateway:$TAG .
docker build -q -f backend/Dockerfile.billing -t llm-gateway/billing:$TAG .
docker build -q -t llm-gateway/web:$TAG web
docker build -q -t llm-gateway/mockupstream:$TAG mockupstream

kind create cluster --name "$CLUSTER" --wait 120s
for img in gateway billing web mockupstream; do
  kind load docker-image "llm-gateway/$img:$TAG" --name "$CLUSTER"
done

kubectl create namespace "$NS"
# Threshold above the $1 top-up so the first call raises a low-balance alert.
helm install llm-gateway deploy/helm/llm-gateway -n "$NS" \
  --set jwtSecret="$(openssl rand -hex 32)" --set adminPassword="$(openssl rand -hex 12)" \
  --set lowBalanceThreshold=2000000 --wait --timeout 8m
kubectl -n "$NS" get pods -o wide

kubectl -n "$NS" port-forward svc/web "$PORT":8080 >/dev/null &
PF_PID=$!
for _ in $(seq 1 30); do curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null && break; sleep 1; done
curl -sf "http://127.0.0.1:$PORT/healthz"; echo

BASE="http://127.0.0.1:$PORT"
EMAIL="smoke-$(date +%s)@example.com"
PW="$(openssl rand -hex 12)"
JAR="$(mktemp)"
curl -sf -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" >/dev/null
curl -sf -c "$JAR" -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" >/dev/null
curl -sf -b "$JAR" -X POST "$BASE/api/billing/topup" -H 'Content-Type: application/json' -d '{"amount_usd":1}'; echo
KEY=$(curl -sf -b "$JAR" -X POST "$BASE/api/keys" -H 'Content-Type: application/json' -d '{"name":"smoke"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["key"])')

echo "--- POST /v1/chat/completions through the cluster"
CODE=$(curl -s -o /tmp/smoke-chat.json -w '%{http_code}' "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello from kind"}]}')
cat /tmp/smoke-chat.json; echo; echo "HTTP $CODE"
[ "$CODE" = "200" ]

echo "--- usage row written asynchronously via Kafka"
for _ in $(seq 1 30); do
  N=$(curl -sf -b "$JAR" "$BASE/api/usage" | python3 -c 'import sys,json; print(json.load(sys.stdin)["count"])')
  [ "$N" -ge 1 ] && break
  sleep 1
done
echo "usage rows: $N"
[ "$N" -ge 1 ]
curl -sf -b "$JAR" "$BASE/api/billing/balance"; echo

echo "--- low-balance alert reached the notifier through RabbitMQ"
for _ in $(seq 1 30); do
  kubectl -n "$NS" logs deploy/notifier | grep -q "LOW BALANCE ALERT" && break
  sleep 1
done
kubectl -n "$NS" logs deploy/notifier | grep "LOW BALANCE ALERT"
echo "SMOKE OK"
