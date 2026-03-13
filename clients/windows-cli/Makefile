.PHONY: fmt test build

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

build:
	GOOS=windows GOARCH=amd64 go build -o ../../dist/windows-cli.exe ./cmd/windows-cli
