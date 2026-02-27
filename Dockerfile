FROM golang:1.21 AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/yourtestsrv cmd/server/main.go

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/yourtestsrv /usr/local/bin/yourtestsrv
COPY config.json /etc/yourtestsrv/config.json

USER nonroot:nonroot

EXPOSE 9000 9001 8080 1883

ENTRYPOINT ["yourtestsrv"]
CMD ["serve-all", "--config", "/etc/yourtestsrv/config.json"]
