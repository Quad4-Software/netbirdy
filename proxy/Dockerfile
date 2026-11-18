# Proxy image. The binary is built by goreleaser (CGO_ENABLED=0) and injected
# into the build context; see .goreleaser.yaml dockers_v2.
#
# Base is Chainguard busybox, the equivalent of the old distroless base:debug.
# It ships CA certificates and a shell, pinned by digest. The netbirdy user is
# appended to the base passwd instead of replacing it, so the image keeps the
# stock accounts alongside the app user.
FROM cgr.dev/chainguard/busybox@sha256:63f6bae487adff606359bd82418ffe0564564a6b272cf52107306749a714afb2
# Chainguard busybox defaults to uid 65532; the chown below needs root.
USER 0:0

RUN echo "netbirdy:x:1000:1000:netbirdy:/var/lib/netbird:/sbin/nologin" >> /etc/passwd && \
    echo "netbirdy:x:1000:" >> /etc/group && \
    mkdir -p /var/lib/netbird /certs && \
    chown -R 1000:1000 /var/lib/netbird /certs && \
    chmod 755 /certs

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-proxy /go/bin/netbirdy-proxy
USER netbirdy:netbirdy
ENV HOME=/var/lib/netbird
ENV NB_PROXY_ADDRESS=":8443"
EXPOSE 8443
ENTRYPOINT ["/go/bin/netbirdy-proxy"]
