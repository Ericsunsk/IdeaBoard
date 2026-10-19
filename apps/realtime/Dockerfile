# syntax=docker/dockerfile:1.7

# Canonical source: root .node-version (enforced by scripts/handbook-version-contract.sh).
# Realtime has no migrator target by design: it is a stateless presence-only
# sidecar with no database schema (ADR-0019), so only the runtime image ships.
ARG NODE_VERSION=22.23.2

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=optional

COPY tsconfig.json ./
COPY src ./src
RUN npm run build

FROM node:${NODE_VERSION}-alpine AS prod-deps

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=dev --omit=optional

FROM node:${NODE_VERSION}-alpine AS runtime-base

RUN apk upgrade --no-cache && \
    rm -rf \
    /usr/local/bin/corepack \
    /usr/local/bin/npm \
    /usr/local/bin/npx \
    /usr/local/lib/node_modules/corepack \
    /usr/local/lib/node_modules/npm

WORKDIR /app

FROM runtime-base AS runtime

ENV NODE_ENV=production \
    RT_HOST=0.0.0.0 \
    RT_PORT=6002

COPY --from=prod-deps --chown=node:node /app/package.json ./
COPY --from=prod-deps --chown=node:node /app/node_modules ./node_modules
COPY --from=builder --chown=node:node /app/dist ./dist

EXPOSE 6002

# The official Node image owns and maintains this named runtime user.
# hadolint ignore=DL3066
USER node

CMD ["node", "dist/index.js"]
