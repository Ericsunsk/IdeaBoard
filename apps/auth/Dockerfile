# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22.22.2
ARG RUNTIME_IMAGE=gcr.io/distroless/nodejs22-debian13:nonroot

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-trixie-slim AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=optional

COPY tsconfig.json ./
COPY src ./src
RUN npm run build

FROM node:${NODE_VERSION}-trixie-slim AS prod-deps

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=dev --omit=optional

FROM ${RUNTIME_IMAGE} AS runtime

WORKDIR /app

ENV NODE_ENV=production \
    AUTH_HOST=0.0.0.0 \
    AUTH_PORT=6001

COPY --from=prod-deps --chown=65532:65532 /app/package.json ./
COPY --from=prod-deps --chown=65532:65532 /app/node_modules ./node_modules
COPY --from=builder --chown=65532:65532 /app/dist ./dist

EXPOSE 6001

USER 65532:65532

CMD ["dist/index.js"]
