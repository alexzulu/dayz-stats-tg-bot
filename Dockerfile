# syntax=docker/dockerfile:1
FROM docker.io/library/golang:1.26.3-alpine AS compiler

COPY . /src
WORKDIR /src

RUN set -x \
    && CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "-s -w" \
      -o ./bot \
      ./cmd/bot/ \
    && ./bot -h \
    && mkdir -p /tmp/rootfs \
    && cd /tmp/rootfs \
    && mkdir -p ./etc/ssl/certs ./bin ./tmp \
    && echo 'appuser:x:10001:10001::/nonexistent:/sbin/nologin' > ./etc/passwd \
    && echo 'appuser:x:10001:' > ./etc/group \
    && cp /etc/ssl/certs/ca-certificates.crt ./etc/ssl/certs/ \
    && chmod 1777 ./tmp \
    && mv /src/bot ./bin/bot

FROM scratch AS runtime

LABEL \
    org.opencontainers.image.title="dayz-stats-tg-bot" \
    org.opencontainers.image.url="https://github.com/alexzulu/dayz-stats-tg-bot" \
    org.opencontainers.image.source="https://github.com/alexzulu/dayz-stats-tg-bot" \
    org.opencontainers.image.licenses="MIT"

COPY --from=compiler /tmp/rootfs /

USER 10001:10001
WORKDIR /tmp
ENTRYPOINT ["/bin/bot"]
