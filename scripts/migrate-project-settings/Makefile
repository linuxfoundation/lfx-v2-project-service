# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT

.PHONY: help run build clean test

help: ## Display available commands
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run the migration script (requires PROJECT_UID)
	@if [ -z "$(PROJECT_UID)" ]; then \
		echo "Usage: make run PROJECT_UID=<project-uid>"; \
		echo "Example: make run PROJECT_UID=7cad5a8d-19d0-41a4-81a6-043453daf9ee"; \
		exit 1; \
	fi
	go run main.go $(PROJECT_UID)

build: ## Build the migration script
	go build -o bin/migrate-project-settings main.go

clean: ## Clean built binaries
	rm -rf bin/

test: ## Run tests
	go test -v ./...

.DEFAULT_GOAL := help