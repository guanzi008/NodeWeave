FROM golang:1.26.1 AS builder

WORKDIR /src
COPY . .

WORKDIR /src/services/relay
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/relay ./cmd/relay

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /out/relay /usr/local/bin/relay

EXPOSE 3478/udp

ENTRYPOINT ["/usr/local/bin/relay"]
