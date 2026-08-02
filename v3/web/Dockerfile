# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS generator
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api/openapi.yaml api/openapi.yaml
COPY cmd/openapi-ts cmd/openapi-ts
RUN go run ./cmd/openapi-ts -spec api/openapi.yaml -out /generated/client.ts

FROM node:22-alpine AS builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
COPY --from=generator /generated/client.ts src/generated/client.ts
RUN npm run typecheck && npm run test && npm exec vite build

FROM nginx:1.27-alpine
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /web/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=12 CMD wget -q -O - http://127.0.0.1:8080/healthz || exit 1
