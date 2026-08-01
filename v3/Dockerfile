# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Version=${VERSION} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Commit=${VCS_REF} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.BuiltAt=${BUILD_DATE}" \
      -o /out/sakura-api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Version=${VERSION} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Commit=${VCS_REF} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.BuiltAt=${BUILD_DATE}" \
      -o /out/sakura-worker ./cmd/worker && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Version=${VERSION} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Commit=${VCS_REF} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.BuiltAt=${BUILD_DATE}" \
      -o /out/sakura-bot ./cmd/bot && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Version=${VERSION} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Commit=${VCS_REF} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.BuiltAt=${BUILD_DATE}" \
      -o /out/sakura-migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Version=${VERSION} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.Commit=${VCS_REF} -X github.com/pjy02/Sakura_embyboss/v3/internal/version.BuiltAt=${BUILD_DATE}" \
      -o /out/sakura-import-v2 ./cmd/import-v2

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 sakura && \
    adduser -S -D -H -u 10001 -G sakura sakura

WORKDIR /app
COPY --from=builder --chown=sakura:sakura /out/sakura-* /app/

USER sakura
STOPSIGNAL SIGTERM
CMD ["/app/sakura-api"]
