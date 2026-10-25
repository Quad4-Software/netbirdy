FROM gcr.io/distroless/base:debug
ENTRYPOINT [ "/go/bin/netbirdy-relay" ]
ENV NB_LOG_FILE=console
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-relay /go/bin/netbirdy-relay
