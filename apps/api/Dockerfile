# syntax=docker/dockerfile:1.7

# Canonical source: apps/api/go.mod 'go' directive (enforced by scripts/handbook-version-contract.sh).
FROM --platform=$BUILDPLATFORM golang:1.26.6-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG ATLAS_VERSION=1.3.2

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    export CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} && \
    go build -trimpath -ldflags="-s -w" -o /out/ideaboard-server ./cmd && \
    go build -trimpath -ldflags="-s -w" -o /out/ideaboard-migrator ./cmd/migrator && \
    go build -trimpath -ldflags="-s -w" -o /out/ideaboard-healthcheck ./cmd/healthcheck && \
    mkdir -p /out/uploads /out/migrations && \
    cp internal/db/migrations/*.sql internal/db/migrations/atlas.sum /out/migrations

RUN apt-get update && \
    apt-get install --no-install-recommends -y \
      ca-certificates="$(apt-cache policy ca-certificates | awk '/Candidate:/ { print $2 }')" \
      curl="$(apt-cache policy curl | awk '/Candidate:/ { print $2 }')" && \
    curl -fsSL "https://release.ariga.io/atlas/atlas-linux-${TARGETARCH}-v${ATLAS_VERSION}" -o /out/atlas && \
    chmod 0555 /out/atlas && \
    rm -rf /var/lib/apt/lists/*

FROM cgr.dev/chainguard/static:latest@sha256:f51c2493951313c3ad4069080b2814ffb6ed6fe3909dabeb84a9482f42d5600b AS runtime

WORKDIR /app

COPY --from=builder --chown=65532:65532 /out/ideaboard-server /app/ideaboard-server
COPY --from=builder --chown=65532:65532 /out/ideaboard-healthcheck /app/ideaboard-healthcheck
COPY --from=builder --chown=65532:65532 /out/uploads /app/uploads

ENV APP_ENV=production \
    APP_HOST=0.0.0.0 \
    APP_PORT=6000 \
    METRICS_ADDR=127.0.0.1:9091

EXPOSE 6000 9091

USER 65532:65532

ENTRYPOINT ["/app/ideaboard-server"]

FROM cgr.dev/chainguard/static:latest-glibc@sha256:f51c2493951313c3ad4069080b2814ffb6ed6fe3909dabeb84a9482f42d5600b AS migrator

WORKDIR /app

COPY --from=builder --chown=65532:65532 /out/ideaboard-migrator /app/ideaboard-migrator
COPY --from=builder --chown=65532:65532 /out/atlas /app/atlas
COPY --from=builder --chown=65532:65532 /out/migrations /app/migrations

USER 65532:65532

ENTRYPOINT ["/app/ideaboard-migrator"]
