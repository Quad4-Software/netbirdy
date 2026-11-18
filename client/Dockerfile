# build & run locally with:
#   cd "$(git rev-parse --show-toplevel)"
#   CGO_ENABLED=0 go build -o netbirdy ./client
#   sudo podman build -t localhost/netbirdy:latest -f client/Dockerfile --ignorefile .dockerignore-client .
#   sudo podman run --rm -it --cap-add={BPF,NET_ADMIN,NET_RAW} localhost/netbirdy:latest

# Wolfi base, pinned by digest: it has apk, a shell and the CA certificate
# bundle. iproute2 provides the ip tooling busybox lacks.
FROM cgr.dev/chainguard/wolfi-base@sha256:e9b8c3739e5fb5fc778843a2ce74f8620588c9ddc9c44918cead527abcb0b628
RUN apk add --no-cache \
    bash \
    ca-certificates-bundle \
    ip6tables \
    iproute2 \
    iptables

ENV \
    NETBIRD_BIN="/usr/local/bin/netbirdy" \
    NB_LOG_FILE="console,/var/log/netbird/client.log" \
    NB_DAEMON_ADDR="unix:///var/run/netbird.sock" \
    NB_ENABLE_CAPTURE="false" \
    NB_ENTRYPOINT_SERVICE_TIMEOUT="30"

ENTRYPOINT [ "/usr/local/bin/netbirdy-entrypoint.sh" ]
ARG TARGETPLATFORM
ARG NETBIRD_BINARY=$TARGETPLATFORM/netbirdy
COPY client/netbirdy-entrypoint.sh /usr/local/bin/netbirdy-entrypoint.sh
COPY "${NETBIRD_BINARY}"  /usr/local/bin/netbirdy
