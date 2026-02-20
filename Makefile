SERVER_BINARY=simple-mcp
CLIENT_BINARY=simple-mcp-cli
GO=go
LDFLAGS=-ldflags="-w -s"
BUILD_ENV=CGO_ENABLED=0 GOOS=linux GOARCH=amd64

.PHONY: all build clean lint-docs test inspect

all: build

build: $(SERVER_BINARY) $(CLIENT_BINARY)

inspect: build
	npx @modelcontextprotocol/inspector ./$(SERVER_BINARY) -t stdio

$(SERVER_BINARY): cmd/simple-mcp/main.go
	$(BUILD_ENV) $(GO) build $(LDFLAGS) -o $(SERVER_BINARY) ./cmd/simple-mcp

$(CLIENT_BINARY): cmd/mcp-cli/main.go
	$(BUILD_ENV) $(GO) build $(LDFLAGS) -o $(CLIENT_BINARY) ./cmd/mcp-cli

clean:
	rm -f $(SERVER_BINARY) $(CLIENT_BINARY)

lint-docs:
	@echo "Linting manpages..."
	@mandoc -Tlint simple-mcp.1
	@mandoc -Tlint simple-mcp-cli.1
	@echo "Linting README and other documentation..."
	@markdownlint-cli2 README.md llm-config-generation/README.md llm-config-generation/system_prompt_mcp_architect.md

test:
	$(GO) test ./...
