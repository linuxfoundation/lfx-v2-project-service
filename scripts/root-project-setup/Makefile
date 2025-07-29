# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

# Makefile for Root Project Setup

.PHONY: build run clean help test deps dev

# Default target
help:
	@echo "Available targets:"
	@echo "  build    - Build the root project setup binary"
	@echo "  run      - Run the root project setup"
	@echo "  clean    - Remove build artifacts"
	@echo "  test     - Run tests (if any)"
	@echo "  deps     - Install dependencies"
	@echo "  dev      - Development mode - build and run with default settings"
	@echo ""
	@echo "Usage examples:"
	@echo "  make build"
	@echo "  make run"
	@echo "  make run NATS_URL=nats://localhost:4222"

# Build the binary
build:
	@echo "Building root project setup..."
	go build -o bin/root-project-setup main.go
	@echo "Binary created: bin/root-project-setup"

# Run the script
run:
	@if [ ! -f "./bin/root-project-setup" ]; then \
		echo "Binary not found. Building first..."; \
		make build; \
	fi
	@echo "Running root project setup..."
	./bin/root-project-setup

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f bin/root-project-setup
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

# Development mode - build and run with default settings
dev:
	@echo "Development mode - building and running..."
	make build
	@echo "Running with default NATS URL (nats://localhost:4222)..."
	./bin/root-project-setup