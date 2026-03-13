.PHONY: fmt test build

fmt:
	gofmt -w ./cmd ./internal

test:
	go test ./...

build:
	GOOS=windows GOARCH=amd64 go build -o ../../dist/windows-agent.exe ./cmd/windows-agent
