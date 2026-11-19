FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/mockupstream .

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
COPY --from=build /out/mockupstream /mockupstream
USER app
EXPOSE 9000
HEALTHCHECK --interval=5s --retries=20 CMD wget -qO- http://127.0.0.1:9000/healthz || exit 1
ENTRYPOINT ["/mockupstream"]
