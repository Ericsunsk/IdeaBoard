# syntax=docker/dockerfile:1.7

FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/ideaboard-server ./cmd

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates potrace \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system ideaboard \
    && useradd --system --gid ideaboard --home-dir /app --shell /usr/sbin/nologin ideaboard \
    && mkdir -p /app/uploads \
    && chown -R ideaboard:ideaboard /app

WORKDIR /app

COPY --from=builder --chown=ideaboard:ideaboard /out/ideaboard-server /app/ideaboard-server

ENV APP_ENV=production \
    APP_GIN_MODE=release \
    APP_HOST=0.0.0.0 \
    APP_PORT=6000 \
    APP_LOG_FILE=none

EXPOSE 6000

USER ideaboard

ENTRYPOINT ["/app/ideaboard-server"]
