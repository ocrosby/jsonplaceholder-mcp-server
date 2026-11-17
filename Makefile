BINARY  := jsonplaceholder-mcp-server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/ocrosby/jsonplaceholder-mcp-server/internal/version.Version=$(VERSION)

.PHONY: build test cover lint vuln run run-http clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race ./...

cover:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run

vuln:
	govulncheck ./...

run: build
	./bin/$(BINARY)

run-http: build
	MCP_TRANSPORT=streamable-http ./bin/$(BINARY)

clean:
	rm -rf bin coverage.out
