SERVER_BINARY=simple-mcp
CLIENT_BINARY=simple-mcp-cli
GO=go
LDFLAGS=-ldflags="-w -s"
BUILD_ENV=CGO_ENABLED=0 GOOS=linux GOARCH=amd64

.PHONY: all build clean lint-docs test

all: build

build: $(SERVER_BINARY) $(CLIENT_BINARY)

$(SERVER_BINARY): main.go config.go executor.go task_store.go scratch.go
	$(BUILD_ENV) $(GO) build $(LDFLAGS) -o $(SERVER_BINARY) .

$(CLIENT_BINARY): cli/main.go
	$(BUILD_ENV) $(GO) build $(LDFLAGS) -o $(CLIENT_BINARY) cli/main.go

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
