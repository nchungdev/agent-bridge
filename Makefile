APP_NAME := nexus-ai
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
	AGENT_HUB_PORT=8088 ./$(BINARY)

docker-build:
	@echo "==> Building Docker image ghcr.io/nchungdev/nexus-ai:latest..."
	docker build -t ghcr.io/nchungdev/nexus-ai:latest .

clean:
	@echo "==> Cleaning build artifacts..."
	rm -f $(BINARY) agent-hub
	rm -rf web/dist
