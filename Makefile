.PHONY: all build build-linux frontend frontend-user frontend-admin run test skill-test clean fmt vet tidy deps docker

BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/hostctl-server
CLI_BIN := $(BIN_DIR)/pagep
LEGACY_CLI_BIN := $(BIN_DIR)/hostctl
MCP_BIN := $(BIN_DIR)/pagep-mcp
LEGACY_MCP_BIN := $(BIN_DIR)/hostctl-mcp

all: build

# Download dependencies.
deps:
	go mod download

# Tidy go.mod/go.sum.
tidy:
	go mod tidy

# Build frontend assets embedded by the Go server.
frontend: frontend-user frontend-admin

frontend-user:
	cd frontend/user && npm install && npm run build

frontend-admin:
	cd frontend/admin && npm install && npm run build

# Build all binaries for the local platform.
build: frontend $(SERVER_BIN) $(CLI_BIN) $(MCP_BIN)

$(SERVER_BIN):
	@mkdir -p $(BIN_DIR)
	go build -o $(SERVER_BIN) ./cmd/hostctl-server

$(CLI_BIN):
	@mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/hostctl
	@cp $(CLI_BIN) $(LEGACY_CLI_BIN)

$(MCP_BIN):
	@mkdir -p $(BIN_DIR)
	go build -o $(MCP_BIN) ./cmd/hostctl-mcp
	@cp $(MCP_BIN) $(LEGACY_MCP_BIN)

# Build Linux amd64 binaries for deployment.
build-linux: frontend
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(SERVER_BIN)-linux-amd64 ./cmd/hostctl-server
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(CLI_BIN)-linux-amd64 ./cmd/hostctl
	@cp $(CLI_BIN)-linux-amd64 $(LEGACY_CLI_BIN)-linux-amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(MCP_BIN)-linux-amd64 ./cmd/hostctl-mcp
	@cp $(MCP_BIN)-linux-amd64 $(LEGACY_MCP_BIN)-linux-amd64

# Run a local dev server.
run: build
	HOSTCTL_DEV=1 $(SERVER_BIN) --addr 127.0.0.1:8787

# Run Go tests.
test:
	go test ./...

# Skill smoke test. Requires a local dev server on 127.0.0.1:8787.
skill-test:
	python test_skill.py

# Format Go code.
fmt:
	gofmt -w .

vet:
	go vet ./...

# Build Docker image.
docker:
	docker build -t hostctl:latest .

# Remove generated local artifacts.
clean:
	rm -rf $(BIN_DIR) data
