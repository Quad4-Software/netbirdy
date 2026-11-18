# Management image. The binary is built by goreleaser (CGO_ENABLED=1 for the
# sqlite store) and injected into the build context; see .goreleaser.yaml
# dockers_v2.
#
# Base is Chainguard glibc-dynamic, pinned by digest: it provides the glibc the
# cgo binary links against plus the CA certificate bundle. The Ubuntu image it
# replaces ran as root, so the same is requested explicitly.
FROM cgr.dev/chainguard/glibc-dynamic@sha256:2c362104c7f7adc37e6bdc87451450ff3834871a6268d930815cbe561493f947
USER 0:0
ENTRYPOINT [ "/go/bin/netbirdy-mgmt","management"]
CMD ["--log-file", "console"]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-mgmt /go/bin/netbirdy-mgmt
