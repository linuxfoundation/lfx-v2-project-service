# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# Project variables
GO_MODULE=github.com/linuxfoundation/lfx-v2-project-service
CMD_PATH=./cmd/project-api
BINARY_NAME=project-api
BINARY_PATH=bin/$(BINARY_NAME)

# API/Code generation variables
API_PATH=$(GO_MODULE)/api/project/v1
DESIGN_MODULE=$(API_PATH)/design
GOA_VERSION=v3.22.6
GO_FILES=$(shell find . -name '*.go' -not -path './api/project/v1/gen/*' -not -path './vendor/*')

# Build variables
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Test variables
TEST_FLAGS=-race
TEST_TIMEOUT=5m

# Docker variables
DOCKER_IMAGE=linuxfoundation/lfx-v2-project-service
DOCKER_TAG=latest

# Helm variables
HELM_CHART_PATH=./charts/lfx-v2-project-service
HELM_RELEASE_NAME=lfx-v2-project-service
HELM_NAMESPACE=lfx
HELM_LOCAL_VALUES_FILE=values.local.yaml

# Default target
.PHONY: all
all: clean deps apigen fmt lint test build

# Help target
.PHONY: help
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
	@echo "  clean          - Remove generated files and binaries"
	@echo "  lint           - Run golangci-lint"
	@echo "  fmt            - Format Go code"
	@echo "  check          - Run fmt, lint, and license check without modifying files"
	@echo "  license-check  - Check for license headers (basic validation)"
	@echo "  verify         - Verify API generation is up to date"
	@echo "  docker-build   - Build Docker image"
	@echo "  helm-install   - Install Helm chart"
	@echo "  helm-install-local - Install Helm chart with local values"
	@echo "  helm-templates   - Print templates for Helm chart"
	@echo "  helm-templates-local - Print templates for Helm chart with local values"
	@echo "  helm-uninstall - Uninstall Helm chart"
	@echo "  helm-restart   - Restart the deployment pod in Kubernetes"

# Install dependencies
.PHONY: deps
deps:
	@echo "==> Installing dependencies..."
	go mod download
	go install goa.design/goa/v3/cmd/goa@$(GOA_VERSION)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "==> Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}

# Generate API code from design files
.PHONY: apigen
apigen: deps
	@echo "==> Generating API code..."
	goa gen $(DESIGN_MODULE) -o api/project/v1
	@echo "==> API generation complete"

# Build the binary
.PHONY: build
build: clean
	@echo "==> Building $(BINARY_NAME)..."
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BINARY_PATH) $(CMD_PATH)
	@echo "==> Build complete: $(BINARY_PATH)"

# Run the service
.PHONY: run
run: apigen
	@echo "==> Running $(BINARY_NAME)..."
	go run $(LDFLAGS) $(CMD_PATH)

# Run with debug logging
.PHONY: debug
debug: apigen
	@echo "==> Running $(BINARY_NAME) in debug mode..."
	go run $(LDFLAGS) $(CMD_PATH) -d

# Run tests
.PHONY: test
test:
	@echo "==> Running tests..."
	go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) ./...

# Run tests with verbose output
.PHONY: test-verbose
test-verbose:
	@echo "==> Running tests (verbose)..."
	go test $(TEST_FLAGS) -v -timeout $(TEST_TIMEOUT) ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "==> Running tests with coverage..."
	@mkdir -p coverage
	go test $(TEST_FLAGS) -cover -timeout $(TEST_TIMEOUT) -coverprofile=coverage/coverage.out ./...
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "==> Coverage report: coverage/coverage.html"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "==> Cleaning build artifacts..."
	@rm -rf bin/ coverage/
	@go clean -cache
	@echo "==> Clean complete"

# Run linter
.PHONY: lint
lint:
	@echo "==> Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Run 'make deps' to install it."; \
		exit 1; \
	fi

# Format code
.PHONY: fmt
fmt:
	@echo "==> Formatting code..."
	@go fmt ./...
	@gofmt -s -w $(GO_FILES)

# Check license headers (basic validation - full check runs in CI)
.PHONY: license-check
license-check:
	@echo "==> Checking license headers (basic validation)..."
	@missing_files=$$(find . -name "*.go" \
		-not -path "./api/project/v1/gen/*" \
		-not -path "./vendor/*" \
		-exec sh -c 'head -10 "$$1" | grep -q "Copyright The Linux Foundation and each contributor to LFX" && head -10 "$$1" | grep -q "SPDX-License-Identifier: MIT" || echo "$$1"' _ {} \;); \
	if [ -n "$$missing_files" ]; then \
		echo "Files missing required license headers:"; \
		echo "$$missing_files"; \
		echo "Required headers:"; \
		echo "  # Copyright The Linux Foundation and each contributor to LFX."; \
		echo "  # SPDX-License-Identifier: MIT"; \
		echo "Note: Full license validation runs in CI"; \
		exit 1; \
	fi
	@echo "==> Basic license header check passed"

# Check formatting and linting without modifying files
.PHONY: check
check:
	@echo "==> Checking code format..."
	@if [ -n "$$(gofmt -l $(GO_FILES))" ]; then \
		echo "The following files need formatting:"; \
		gofmt -l $(GO_FILES); \
		exit 1; \
	fi
	@echo "==> Code format check passed"
	@$(MAKE) lint
	@$(MAKE) license-check

# Verify that generated code is up to date
.PHONY: verify
verify: apigen
	@echo "==> Verifying generated code is up to date..."
	@if [ -n "$$(git status --porcelain api/project/v1/gen/)" ]; then \
		echo "Generated code is out of date. Run 'make apigen' and commit the changes."; \
		git status --porcelain api/project/v1/gen/; \
		exit 1; \
	fi
	@echo "==> Generated code is up to date"

# Build Docker image
.PHONY: docker-build
docker-build:
	@echo "==> Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f Dockerfile .
	@echo "==> Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Install Helm chart
.PHONY: helm-install
helm-install:
	@echo "==> Installing Helm chart..."
	helm upgrade --force --install $(HELM_RELEASE_NAME) $(HELM_CHART_PATH) --namespace $(HELM_NAMESPACE)
	@echo "==> Helm chart installed: $(HELM_RELEASE_NAME)"

# Install Helm chart with local values
.PHONY: helm-install-local
helm-install-local:
	@echo "==> Installing Helm chart..."
	helm upgrade --force --install $(HELM_RELEASE_NAME) $(HELM_CHART_PATH) --namespace $(HELM_NAMESPACE) --values $(HELM_CHART_PATH)/$(HELM_LOCAL_VALUES_FILE)
	@echo "==> Helm chart installed: $(HELM_RELEASE_NAME)"

# Print templates for Helm chart
.PHONY: helm-templates
helm-templates:
	@echo "==> Printing templates for Helm chart..."
	helm template $(HELM_RELEASE_NAME) $(HELM_CHART_PATH) --namespace $(HELM_NAMESPACE)
	@echo "==> Templates printed for Helm chart: $(HELM_RELEASE_NAME)"

# Print templates for Helm chart with local values
.PHONY: helm-templates-local
helm-templates-local:
	@echo "==> Printing templates for Helm chart..."
	helm template $(HELM_RELEASE_NAME) $(HELM_CHART_PATH) --namespace $(HELM_NAMESPACE) --values $(HELM_CHART_PATH)/$(HELM_LOCAL_VALUES_FILE)
	@echo "==> Templates printed for Helm chart: $(HELM_RELEASE_NAME)"

# Uninstall Helm chart
.PHONY: helm-uninstall
helm-uninstall:
	@echo "==> Uninstalling Helm chart..."
	helm uninstall $(HELM_RELEASE_NAME) --namespace $(HELM_NAMESPACE)
	@echo "==> Helm chart uninstalled: $(HELM_RELEASE_NAME)"

# Restart the deployment pod
.PHONY: helm-restart
helm-restart:
	@echo "==> Restarting deployment pod..."
	kubectl rollout restart deployment/$(HELM_RELEASE_NAME) --namespace $(HELM_NAMESPACE)
	@echo "==> Deployment restarted: $(HELM_RELEASE_NAME)"
