.PHONY: fmt test run-controlplane build

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

run-controlplane:
	go run ./cmd/controlplane

build:
	go build -o ../../dist/controlplane ./cmd/controlplane
