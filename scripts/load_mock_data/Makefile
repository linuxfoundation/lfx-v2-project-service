# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# Makefile for Project Mock Data Loader

.PHONY: build run clean help test

# Default target
help:
	@echo "Available targets:"
	@echo "  build    - Build the mock data loader binary"
	@echo "  run      - Run the mock data loader (requires -bearer-token)"
	@echo "  clean    - Remove build artifacts"
	@echo "  test     - Run tests (if any)"
	@echo ""
	@echo "Usage examples:"
	@echo "  make build"
	@echo "  make run ARGS='-bearer-token \"your-token\" -num-projects 5'"
	@echo "  make run ARGS='-bearer-token \"your-token\" -api-url \"http://localhost:8080/projects\"'"

# Build the binary
build:
	@echo "Building mock data loader..."
	go build -o bin/load_mock_data main.go
	@echo "Binary created: bin/load_mock_data"

# Run the script
run:
	@if [ -z "$(ARGS)" ]; then \
		echo "Error: ARGS variable is required for run target"; \
		echo "Example: make run ARGS='-bearer-token \"your-token\" -num-projects 5'"; \
		exit 1; \
	fi
	@if [ ! -f "./bin/load_mock_data" ]; then \
		echo "Binary not found. Building first..."; \
		make build; \
	fi
	@echo "Running mock data loader with args: $(ARGS)"
	./bin/load_mock_data $(ARGS)

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f bin/load_mock_data
	@echo "Clean complete"

# Run tests (placeholder for future tests)
test:
	@echo "Running tests..."
	@echo "No tests implemented yet"
	@echo "Tests complete"

# Install dependencies (if needed)
deps:
	@echo "Installing dependencies..."
	go mod tidy
	@echo "Dependencies installed"

# Development mode - build and run with common args
dev:
	@echo "Development mode - building and running with sample args..."
	make build
	@echo "To run with your token:"
	@echo "  ./bin/load_mock_data -bearer-token \"your-jwt-token\" -num-projects 5" 
