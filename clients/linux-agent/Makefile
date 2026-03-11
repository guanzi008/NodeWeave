.PHONY: fmt test build

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

build:
	go build -o ../../dist/linux-agent ./cmd/linux-agent

