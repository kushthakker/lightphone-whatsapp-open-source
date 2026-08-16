FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/bridge ./cmd/bridge

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 bridge \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin bridge \
    && install -d --owner=bridge --group=bridge --mode=0700 /data

COPY --from=builder /out/bridge /usr/local/bin/bridge

USER 10001:10001
ENV LISTEN_ADDR=0.0.0.0:8080
ENV DATABASE_PATH=/data/bridge.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/usr/local/bin/bridge", "-healthcheck", "http://127.0.0.1:8080/healthz"]

ENTRYPOINT ["/usr/local/bin/bridge"]
