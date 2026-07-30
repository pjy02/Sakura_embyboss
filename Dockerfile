FROM node:22-alpine AS web-builder

WORKDIR /web
COPY web/package*.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build


FROM python:3.10.11-alpine AS python-builder

RUN apk add --no-cache --virtual .build-deps gcc musl-dev openssl-dev coreutils
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN find . -type f -name "*.pyc" -delete
RUN apk del --purge .build-deps
RUN rm -rf /tmp/* /root/.cache /var/cache/apk/*


FROM python:3.10.11-alpine

ENV TZ=Asia/Shanghai \
    DOCKER_MODE=1 \
    PYTHONUNBUFFERED=1 \
    WORKDIR=/app

RUN apk add --no-cache \
    mariadb-connector-c \
    tzdata \
    mysql-client \
    git && \
    ln -snf Asia/Shanghai /etc/localtime && echo Asia/Shanghai > /etc/timezone

WORKDIR ${WORKDIR}

COPY --from=python-builder /usr/local/lib/python3.10/site-packages /usr/local/lib/python3.10/site-packages
COPY --from=python-builder /usr/local/bin /usr/local/bin
COPY . .
COPY --from=web-builder /web/dist ./web/dist

# Only the default Bot picture is needed at runtime; Web assets live under web/dist.
RUN find ./image -type f ! -name "bot2.png" -delete

ENTRYPOINT ["python3"]
CMD ["main.py"]
