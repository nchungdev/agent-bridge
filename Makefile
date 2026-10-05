APP_NAME := agent-bridge
BINARY := $(APP_NAME)
GO := /usr/local/go/bin/go
ifeq ($(wildcard $(GO)),)
	GO := go
endif

.PHONY: all build build-web build-go run clean docker-build

all: build

build-web:
	@echo "==> Building React web bundle..."
	cd web && npm run build

build-go:
	@echo "==> Building Go binary..."
	$(GO) build -ldflags="-w -s" -o $(BINARY) .

build: build-web build-go
	@echo "==> Build complete: ./$(BINARY)"

run: build
	@echo "==> Starting $(APP_NAME)..."
	AGENT_BRIDGE_PORT=8088 ./$(BINARY)

docker-build:
	@echo "==> Building Docker image ghcr.io/nchungdev/agent-bridge:latest..."
	docker build -t ghcr.io/nchungdev/agent-bridge:latest .

clean:
	@echo "==> Cleaning build artifacts..."
	rm -f $(BINARY) agent-bridge
	rm -rf web/dist
