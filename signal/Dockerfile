# Signal server image. The binary is built by goreleaser (CGO_ENABLED=0) and
# injected into the build context; see .goreleaser.yaml dockers_v2.
#
# Base is Chainguard busybox, the equivalent of the old distroless base:debug:
# a shell and core applets for debugging, CA certificates included, pinned by
# digest. The image runs as root like the distroless one did.
FROM cgr.dev/chainguard/busybox@sha256:63f6bae487adff606359bd82418ffe0564564a6b272cf52107306749a714afb2
USER 0:0
ENTRYPOINT [ "/go/bin/netbirdy-signal","run" ]
CMD ["--log-file", "console"]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-signal /go/bin/netbirdy-signal
