#!/usr/bin/env bash
# Regenerates backend/internal/billingpb from proto/billing.proto (needs Docker).
# Generated files are committed, so this only runs when the proto changes.
set -euo pipefail
cd "$(dirname "$0")/.."
docker run --rm -e UID_GID="$(id -u):$(id -g)" -v "$PWD:/src" -w /src golang:1.27 bash -c '
  set -e
  apt-get update -qq && apt-get install -y -qq protobuf-compiler >/dev/null
  GOBIN=/usr/local/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
  GOBIN=/usr/local/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
  mkdir -p backend/internal/billingpb
  protoc -I proto --go_out=backend/internal/billingpb --go_opt=paths=source_relative \
    --go-grpc_out=backend/internal/billingpb --go-grpc_opt=paths=source_relative proto/billing.proto
  chown -R "$UID_GID" backend/internal/billingpb
'
echo "generated backend/internal/billingpb"
