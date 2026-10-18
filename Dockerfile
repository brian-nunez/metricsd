# syntax=docker/dockerfile:1

# Builds a minimal image holding the two static binaries. It is meant to be
# copied into application images, so metricsd runs inside the container it
# measures:
#
#   docker build -t metricsd:latest .
#   COPY --from=metricsd:latest /usr/local/bin/ /usr/local/bin/
#
# Running this image on its own measures only this image's container.

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /out/ ./cmd/... \
 && mkdir -p /rootfs/tmp && chmod 1777 /rootfs/tmp

FROM scratch
# CA roots for OTLP over HTTPS, and a world-writable /tmp for the default
# database and socket when the container runs as a non-root user. /tmp is
# copied as an entry of a tree because COPY of a directory itself drops
# its mode.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /rootfs/ /
COPY --from=build /out/metricsd /out/metricsctl /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/metricsd"]
