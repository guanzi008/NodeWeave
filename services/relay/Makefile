.PHONY: fmt test build run-relay

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

build:
	go build -o ../../dist/relay ./cmd/relay

run-relay:
	go run ./cmd/relay
