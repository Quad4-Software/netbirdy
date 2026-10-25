FROM gcr.io/distroless/base:debug
ENTRYPOINT [ "/go/bin/netbirdy-signal","run" ]
CMD ["--log-file", "console"]
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/netbirdy-signal /go/bin/netbirdy-signal
