# syntax=docker/dockerfile:1.7

# Canonical source: apps/api/go.mod 'go' directive (enforced by scripts/handbook-version-contract.sh).
FROM --platform=$BUILDPLATFORM golang:1.26.11-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG ATLAS_VERSION=1.3.11-ede06dc-canary

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /src

COPY go.mod go.sum ./
COPY internal/testdb/go.mod internal/testdb/go.sum ./internal/testdb/
COPY internal/testsupport/go.mod internal/testsupport/go.sum ./internal/testsupport/
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
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
    ./install-atlas.sh /out/atlas "${TARGETARCH}" && \
    rm -rf /var/lib/apt/lists/*

FROM cgr.dev/chainguard/static:latest@sha256:fc42af33e643f20d08baf03cddcc1c02ba57052dfb02edb0d79f3fc47bcea8d9 AS runtime

WORKDIR /app

COPY --from=builder --chown=65532:65532 /out/ideaboard-server /app/ideaboard-server
COPY --from=builder --chown=65532:65532 /out/ideaboard-healthcheck /app/ideaboard-healthcheck
COPY --from=builder --chown=65532:65532 /out/uploads /app/uploads

ENV APP_ENV=production

EXPOSE 6000 9091

USER 65532:65532

ENTRYPOINT ["/app/ideaboard-server"]

FROM cgr.dev/chainguard/static:latest-glibc@sha256:fc42af33e643f20d08baf03cddcc1c02ba57052dfb02edb0d79f3fc47bcea8d9 AS migrator

WORKDIR /app

COPY --from=builder --chown=65532:65532 /out/ideaboard-migrator /app/ideaboard-migrator
COPY --from=builder --chown=65532:65532 /out/atlas /app/atlas
COPY --from=builder --chown=65532:65532 /out/migrations /app/migrations

USER 65532:65532

ENTRYPOINT ["/app/ideaboard-migrator"]
