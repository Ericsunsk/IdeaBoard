# syntax=docker/dockerfile:1.7

# Canonical source: root .node-version (enforced by scripts/handbook-version-contract.sh).
ARG NODE_VERSION=22.23.5
# Canonical production runtime: root .bun-image (same fallback contract).
ARG BUN_IMAGE=oven/bun:1.4.5-alpine@sha256:da816670775f4c2b4e990b6b1847cf5540dcdc721a42033dd331ce7a88ba6a3b

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=optional

COPY tsconfig.json ./
COPY scripts ./scripts
COPY src ./src
RUN npm run build

FROM node:${NODE_VERSION}-alpine AS prod-deps

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=dev --omit=optional

FROM ${BUN_IMAGE} AS runtime-base

# Bun embeds BoringSSL. Keep the CA bundle; remove the unused package manager
# and its OpenSSL/CLI dependency chain from the production runtime.
# Resolve CA and OS security updates together; the runtime base is digest-pinned.
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates-bundle && \
    apk upgrade --no-cache && \
    apk del --no-cache apk-tools libapk ssl_client scanelf libcrypto3 libssl3

WORKDIR /app

FROM runtime-base AS runtime

ENV NODE_ENV=production \
    AUTH_HOST=0.0.0.0 \
    AUTH_PORT=6001

COPY --from=prod-deps --chown=bun:bun /app/package.json ./
COPY --from=prod-deps --chown=bun:bun /app/node_modules ./node_modules
COPY --from=builder --chown=bun:bun /app/dist ./dist

EXPOSE 6001

# The official Bun image owns and maintains this named runtime user.
# hadolint ignore=DL3066
USER bun

CMD ["bun", "dist/index.js"]

FROM runtime-base AS migrator

ENV NODE_ENV=production

COPY --from=prod-deps --chown=bun:bun /app/package.json ./
COPY --from=prod-deps --chown=bun:bun /app/node_modules ./node_modules
COPY --from=builder --chown=bun:bun /app/dist ./dist

# The official Bun image owns and maintains this named runtime user.
# hadolint ignore=DL3066
USER bun

CMD ["bun", "dist/migrate.js"]
