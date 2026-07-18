# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=experimental
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY scripts/build.sh ./scripts/build.sh
RUN CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    VERSION="$VERSION" COMMIT="$COMMIT" BUILD_DATE="$BUILD_DATE" \
    OUTPUT=/out/accelerator sh scripts/build.sh

FROM alpine:3.22

ARG VERSION=experimental
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Database Accelerator" \
      org.opencontainers.image.description="Experimental single-upstream MySQL/MariaDB connection accelerator" \
      org.opencontainers.image.source="https://github.com/podopodo/db_accelerator" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$BUILD_DATE" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 accelerator \
    && adduser -S -D -H -u 10001 -G accelerator accelerator \
    && mkdir -p /etc/database-accelerator /var/lib/database-accelerator \
    && chown -R accelerator:accelerator /var/lib/database-accelerator
COPY --from=build /out/accelerator /usr/local/bin/accelerator
COPY deploy/accelerator.container.yaml /etc/database-accelerator/accelerator.yaml

USER accelerator:accelerator
WORKDIR /var/lib/database-accelerator
EXPOSE 3307 9090
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/accelerator", "healthcheck", "--url", "http://127.0.0.1:9090/readyz"]
ENTRYPOINT ["/usr/local/bin/accelerator"]
CMD ["serve", "--config", "/etc/database-accelerator/accelerator.yaml"]
