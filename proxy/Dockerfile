FROM golang:1.27.2-alpine AS builder
WORKDIR /app

RUN echo "netbirdy:x:1000:1000:netbirdy:/var/lib/netbird:/sbin/nologin" > /tmp/passwd && \
    echo "netbirdy:x:1000:netbirdy" > /tmp/group && \
    mkdir -p /tmp/var/lib/netbird && \
    mkdir -p /tmp/certs

FROM gcr.io/distroless/base:debug
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-proxy /go/bin/netbirdy-proxy
COPY --from=builder /tmp/passwd /etc/passwd
COPY --from=builder /tmp/group /etc/group
COPY --from=builder --chown=1000:1000 /tmp/var/lib/netbird /var/lib/netbird
COPY --from=builder --chown=1000:1000 --chmod=755 /tmp/certs /certs
USER netbirdy:netbirdy
ENV HOME=/var/lib/netbird
ENV NB_PROXY_ADDRESS=":8443"
EXPOSE 8443
ENTRYPOINT ["/go/bin/netbirdy-proxy"]
