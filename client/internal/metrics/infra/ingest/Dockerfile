FROM cgr.dev/chainguard/go@sha256:78352255a8760797dac56865a5b9af2881f245ef74c90199e4e15f52251c5357 AS build
WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o ingest .

FROM cgr.dev/chainguard/wolfi-base@sha256:e9b8c3739e5fb5fc778843a2ce74f8620588c9ddc9c44918cead527abcb0b628
RUN adduser -D -H ingest
COPY --from=build /app/ingest /usr/local/bin/ingest
USER ingest
ENTRYPOINT ["ingest"]
