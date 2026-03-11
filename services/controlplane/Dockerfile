FROM golang:1.26.1-alpine AS build

WORKDIR /src

COPY go.work ./
COPY packages/contracts/go ./packages/contracts/go
COPY services/controlplane ./services/controlplane

WORKDIR /src/services/controlplane
RUN go mod download
RUN go build -trimpath -ldflags="-s -w" -o /out/controlplane ./cmd/controlplane

FROM alpine:3.22

RUN adduser -D -u 10001 app && \
    mkdir -p /app /data && \
    chown -R app:app /app /data

WORKDIR /app
COPY --from=build /out/controlplane /usr/local/bin/controlplane

ENV CONTROLPLANE_ADDRESS=:8080
ENV CONTROLPLANE_STORAGE_DRIVER=sqlite
ENV CONTROLPLANE_SQLITE_PATH=/data/controlplane.db

USER app

EXPOSE 8080

ENTRYPOINT ["controlplane"]

