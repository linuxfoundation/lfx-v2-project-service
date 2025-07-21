# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# Variables
BINARY_NAME=project-api
BINARY_PATH=bin/$(BINARY_NAME)
GO_MODULE=github.com/linuxfoundation/lfx-v2-project-service
CMD_PATH=$(GO_MODULE)/cmd/project-api
DESIGN_MODULE=$(CMD_PATH)/design
GO_FILES=$(shell find . -name '*.go' -not -path './gen/*' -not -path './vendor/*')
GOA_VERSION=v3

# Docker variables
DOCKER_IMAGE=linuxfoundation/lfx-v2-project-service
DOCKER_TAG=0.1.0

# Helm variables
HELM_CHART_PATH=../../charts/lfx-v2-project-service
HELM_RELEASE_NAME=lfx-v2-project-service
HELM_NAMESPACE=lfx

# Build variables
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Test variables
TEST_FLAGS=-v -race -cover
TEST_TIMEOUT=5m

.PHONY: all help deps apigen build run debug test test-verbose test-coverage test-integration test-authorization clean lint fmt check verify docker helm-install helm-uninstall

# Default target
all: clean deps apigen fmt lint test build

# Help target
help:
	@echo "Available targets:"
	@echo "  all            - Run clean, deps, apigen, fmt, lint, test, and build"
	@echo "  deps           - Install dependencies including goa CLI"
	@echo "  apigen         - Generate API code from design files"
	@echo "  build          - Build the binary"
	@echo "  run            - Run the service"
	@echo "  debug          - Run the service with debug logging"
	@echo "  test           - Run unit tests"
	@echo "  test-verbose   - Run tests with verbose output"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  test-integration - Run integration tests (requires build tags)"
	@echo "  test-authorization - Run authorization integration test script"
	@echo "  clean          - Remove generated files and binaries"
	@echo "  lint           - Run golangci-lint"
	@echo "  fmt            - Format Go code"
	@echo "  check          - Run fmt and lint without modifying files"
	@echo "  verify         - Verify API generation is up to date"
	@echo "  docker         - Build Docker image"
	@echo "  helm-install   - Install Helm chart"
	@echo "  helm-uninstall - Uninstall Helm chart"

# Install dependencies
deps:
	@echo "==> Installing dependencies..."
	go mod download
	go install goa.design/goa/$(GOA_VERSION)/cmd/goa@latest
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "==> Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}

# Generate API code from design files
apigen: deps
	@echo "==> Generating API code..."
	goa gen $(DESIGN_MODULE)
	@echo "==> API generation complete"

# Build the binary
build: clean
	@echo "==> Building $(BINARY_NAME)..."
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BINARY_PATH) .
	@echo "==> Build complete: $(BINARY_PATH)"

# Run the service
run: apigen
	@echo "==> Running $(BINARY_NAME)..."
	go run $(LDFLAGS) .

# Run with debug logging
debug: apigen
	@echo "==> Running $(BINARY_NAME) in debug mode..."
	go run $(LDFLAGS) . -d

# Run tests
test:
	@echo "==> Running tests..."
	go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) ./...

# Run tests with verbose output
test-verbose:
	@echo "==> Running tests (verbose)..."
	go test $(TEST_FLAGS) -v -timeout $(TEST_TIMEOUT) ./...

# Run tests with coverage
test-coverage:
	@echo "==> Running tests with coverage..."
	@mkdir -p coverage
	go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) -coverprofile=coverage/coverage.out ./...
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "==> Coverage report: coverage/coverage.html"

# Run integration tests
test-integration:
	@echo "==> Running integration tests..."
	go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) -tags=integration ./...

# Run authorization integration test script
test-authorization:
	@echo "==> Running authorization integration tests..."
	@if [ ! -f ../../test/authorization_test.sh ]; then \
		echo "Authorization test script not found at ../../test/authorization_test.sh"; \
		exit 1; \
	fi
	@../../test/authorization_test.sh

# Clean build artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	@rm -rf bin/ coverage/
	@go clean -cache
	@echo "==> Clean complete"

# Run linter
lint:
	@echo "==> Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Run 'make deps' to install it."; \
		exit 1; \
	fi

# Format code
fmt:
	@echo "==> Formatting code..."
	@go fmt ./...
	@gofmt -s -w $(GO_FILES)

# Check formatting and linting without modifying files
check:
	@echo "==> Checking code format..."
	@if [ -n "$$(gofmt -l $(GO_FILES))" ]; then \
		echo "The following files need formatting:"; \
		gofmt -l $(GO_FILES); \
		exit 1; \
	fi
	@echo "==> Code format check passed"
	@$(MAKE) lint

# Verify that generated code is up to date
verify: apigen
	@echo "==> Verifying generated code is up to date..."
	@if [ -n "$$(git status --porcelain gen/)" ]; then \
		echo "Generated code is out of date. Run 'make apigen' and commit the changes."; \
		git status --porcelain gen/; \
		exit 1; \
	fi
	@echo "==> Generated code is up to date"

# Build Docker image
docker:
	@echo "==> Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f ../../Dockerfile ../../
	@echo "==> Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Install Helm chart
helm-install:
	@echo "==> Installing Helm chart..."
	helm upgrade --install $(HELM_RELEASE_NAME) $(HELM_CHART_PATH) --namespace $(HELM_NAMESPACE)
	@echo "==> Helm chart installed: $(HELM_RELEASE_NAME)"

helm-install-reuse:
	@echo "==> Installing Helm chart with reuse..."
	helm upgrade --install $(HELM_RELEASE_NAME) $(HELM_CHART_PATH) --namespace $(HELM_NAMESPACE) --reuse-values
	@echo "==> Helm chart installed: $(HELM_RELEASE_NAME)"

# Uninstall Helm chart
helm-uninstall:
	@echo "==> Uninstalling Helm chart..."
	helm uninstall $(HELM_RELEASE_NAME) --namespace $(HELM_NAMESPACE)
	@echo "==> Helm chart uninstalled: $(HELM_RELEASE_NAME)"
