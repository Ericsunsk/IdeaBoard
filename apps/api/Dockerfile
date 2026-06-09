# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26.4-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/ideaboard-server ./cmd && \
    go build -trimpath -ldflags="-s -w" -o /out/ideaboard-healthcheck ./cmd/healthcheck && \
    mkdir -p /out/uploads

FROM cgr.dev/chainguard/static:latest-glibc AS runtime

WORKDIR /app

COPY --from=builder --chown=65532:65532 /out/ideaboard-server /app/ideaboard-server
COPY --from=builder --chown=65532:65532 /out/ideaboard-healthcheck /app/ideaboard-healthcheck
COPY --from=builder --chown=65532:65532 /out/uploads /app/uploads

ENV APP_ENV=production \
    APP_GIN_MODE=release \
    APP_HOST=0.0.0.0 \
    APP_PORT=6000 \
    APP_LOG_FILE=none

EXPOSE 6000

USER 65532:65532

ENTRYPOINT ["/app/ideaboard-server"]
