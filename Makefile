.PHONY: build run clean test setup generate-token help

BINARY_NAME=tunnelab-server
CONFIG_PATH=configs/server.yaml

help:
	@echo "TunneLab Server - Makefile Commands"
	@echo "===================================="
	@echo "  make build          - Build the server binary"
	@echo "  make run            - Run the server"
	@echo "  make setup          - Initial setup (config + build + db)"
	@echo "  make generate-token - Generate a new client token"
	@echo "  make test           - Run tests"
	@echo "  make clean          - Clean build artifacts"
	@echo ""

build:
	@echo "Building TunneLab server..."
	@go build -ldflags="-s -w" -trimpath -o $(BINARY_NAME) ./cmd/server
	@echo "✓ Build complete: $(BINARY_NAME)"

run: build
	@echo "Starting TunneLab server..."
	@./$(BINARY_NAME) -config $(CONFIG_PATH)

setup:
	@echo "Running setup..."
	@chmod +x scripts/setup.sh scripts/generate-token.sh
	@./scripts/setup.sh

generate-token:
	@chmod +x scripts/generate-token.sh
	@./scripts/generate-token.sh

test:
	@echo "Running tests..."
	@go test -v ./...

clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -f *.db *.db-shm *.db-wal
	@echo "✓ Clean complete"

deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies updated"

install:
	@echo "Installing TunneLab server..."
	@go install ./cmd/server
	@echo "✓ Installed to $(shell go env GOPATH)/bin/server"
