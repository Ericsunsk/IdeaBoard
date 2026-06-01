# syntax=docker/dockerfile:1.7

FROM node:22.18.0-alpine AS node-base

ARG NPM_VERSION=11.16.0
RUN npm install -g "npm@${NPM_VERSION}"

FROM node-base AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY tsconfig.json ./
COPY src ./src
RUN npm run build

FROM node-base AS runtime

WORKDIR /app

ENV NODE_ENV=production \
    AUTH_HOST=0.0.0.0 \
    AUTH_PORT=6001

COPY --from=builder --chown=node:node /app/package.json /app/package-lock.json ./
COPY --from=builder --chown=node:node /app/node_modules ./node_modules
COPY --from=builder --chown=node:node /app/dist ./dist

EXPOSE 6001

USER node

CMD ["node", "dist/index.js"]
