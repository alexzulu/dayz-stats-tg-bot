# syntax=docker/dockerfile:1
FROM docker.io/library/golang:1.27.0-alpine AS compiler

COPY . /src
WORKDIR /src

RUN set -x \
    && go generate ./... \
    && CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w" \
      -o ./tg-bot \
      ./cmd/tg-bot/ \
    && ./tg-bot -h \
    && mkdir -p /tmp/rootfs \
    && cd /tmp/rootfs \
    && mkdir -p ./etc/ssl/certs ./bin ./tmp ./data \
    && echo 'appuser:x:10001:10001::/data:/sbin/nologin' > ./etc/passwd \
    && echo 'appuser:x:10001:' > ./etc/group \
    && cp /etc/ssl/certs/ca-certificates.crt ./etc/ssl/certs/ \
    && chmod 1777 ./tmp \
    && chown 10001:10001 ./data \
    && chmod 0700 ./data \
    && mv /src/tg-bot ./bin/tg-bot

FROM scratch AS runtime

LABEL \
    org.opencontainers.image.title="dayz-stats-tg-bot" \
    org.opencontainers.image.url="https://github.com/alexzulu/dayz-stats-tg-bot" \
    org.opencontainers.image.source="https://github.com/alexzulu/dayz-stats-tg-bot" \
    org.opencontainers.image.licenses="MIT"

COPY --from=compiler /tmp/rootfs /

USER 10001:10001
WORKDIR /data
ENV CONFIG_FILE="/data/config.json"
VOLUME ["/data"]
ENTRYPOINT ["/bin/tg-bot"]
