# Build context: repository root (needs docs/openapi.yaml).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG COMMIT=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.commit=${COMMIT}" -o /out/gateway ./cmd/gateway

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/gateway /app/gateway
COPY docs/openapi.yaml /app/docs/openapi.yaml
USER app
EXPOSE 8080
HEALTHCHECK --interval=5s --retries=20 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/gateway"]
