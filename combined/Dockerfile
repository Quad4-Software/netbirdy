FROM ubuntu:24.04
RUN apt update && apt install -y ca-certificates && rm -fr /var/cache/apt
ENTRYPOINT [ "/go/bin/netbirdy-server" ]
CMD ["--config", "/etc/netbird/config.yaml"]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-server /go/bin/netbirdy-server
