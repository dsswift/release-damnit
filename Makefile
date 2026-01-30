.PHONY: build test test-short test-integration test-e2e clean lint fmt coverage

# Build settings
BINARY_NAME=release-damnit
BUILD_DIR=build
VERSION=$(shell cat VERSION 2>/dev/null || echo "0.0.0")
GIT_SHA=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.gitSha=${GIT_SHA}"

# Go settings
GOFLAGS=-mod=readonly

# Default target
all: test build

# Build binary
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/release-damnit

# Build for all platforms
build-all:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/release-damnit
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/release-damnit
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/release-damnit
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/release-damnit

# Run all tests
test:
	go test -v ./...

# Run unit tests only (fast)
test-short:
	go test -v -short ./...

# Run integration tests
test-integration:
	go test -v -run Integration ./...

# Run E2E tests (requires mock repo access)
test-e2e:
	go test -v -tags=e2e ./e2e/...

# Generate coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .

# Lint code
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Install binary locally
install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Verify module
verify:
	go mod verify
	go mod tidy

# Run the tool (dry run)
run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --dry-run

# Help
help:
	@echo "Available targets:"
	@echo "  build           - Build binary for current platform"
	@echo "  build-all       - Build binaries for all platforms"
	@echo "  test            - Run all tests"
	@echo "  test-short      - Run unit tests only (fast)"
	@echo "  test-integration - Run integration tests"
	@echo "  test-e2e        - Run E2E tests"
	@echo "  coverage        - Generate coverage report"
	@echo "  fmt             - Format code"
	@echo "  lint            - Lint code"
	@echo "  clean           - Clean build artifacts"
	@echo "  install         - Install binary to /usr/local/bin"
	@echo "  verify          - Verify module dependencies"
	@echo "  run             - Build and run with --dry-run"
