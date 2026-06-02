# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22.22.2

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

FROM node:${NODE_VERSION}-alpine AS runtime

WORKDIR /app

ENV NODE_ENV=production \
    AUTH_HOST=0.0.0.0 \
    AUTH_PORT=6001

COPY --from=prod-deps --chown=node:node /app/package.json ./
COPY --from=prod-deps --chown=node:node /app/node_modules ./node_modules
COPY --from=builder --chown=node:node /app/dist ./dist

EXPOSE 6001

USER node

CMD ["node", "dist/index.js"]
