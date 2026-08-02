# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22
ARG PYTHON_VERSION=3.10

FROM node:${NODE_VERSION}-alpine AS web-builder

WORKDIR /web
COPY web/package.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm install --no-audit --no-fund
COPY web/ ./
RUN npm run typecheck && npm run test && npm run build


FROM python:${PYTHON_VERSION}-alpine AS python-builder

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PATH=/opt/venv/bin:${PATH}

RUN apk add --no-cache --virtual .build-deps \
    build-base \
    cargo \
    coreutils \
    freetype-dev \
    jpeg-dev \
    libffi-dev \
    openssl-dev \
    zlib-dev
RUN python -m venv /opt/venv
COPY requirements.lock ./
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install -r requirements.lock && \
    pip check


FROM python:${PYTHON_VERSION}-alpine AS runtime

ARG BUILD_DATE=""
ARG VERSION="dev"
ARG VCS_REF=""

LABEL org.opencontainers.image.title="Sakura EmbyBoss" \
      org.opencontainers.image.description="Telegram Bot and Web management center for Emby" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"

ENV TZ=Asia/Shanghai \
    DOCKER_MODE=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH=/opt/venv/bin:${PATH} \
    WORKDIR=/app

RUN apk add --no-cache \
    freetype \
    git \
    libffi \
    libjpeg-turbo \
    libstdc++ \
    mariadb-client \
    mariadb-connector-c \
    openssl \
    tini \
    tzdata \
    zlib && \
    ln -snf /usr/share/zoneinfo/${TZ} /etc/localtime && \
    echo "${TZ}" > /etc/timezone

WORKDIR ${WORKDIR}

COPY --from=python-builder /opt/venv /opt/venv
COPY . .
COPY --from=web-builder /web/dist ./web/dist

RUN mkdir -p ./log ./db_backup && \
    find ./image -type f ! -name "bot2.png" -delete && \
    python -m compileall -q backend bot scripts/container_healthcheck.py

STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=45s --retries=3 \
    CMD ["python3", "scripts/container_healthcheck.py"]

ENTRYPOINT ["/sbin/tini", "--", "python3"]
CMD ["main.py"]
