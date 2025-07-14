# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
.PHONY: deps apigen build run debug test

# Install the goa CLI tool.
deps:
	go install goa.design/goa/v3/cmd/goa@latest

# Generate the API code from the design files.
apigen: deps
	goa gen github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/design

# Build the go program to ensure there aren't any build errors.
build:
	go build -o bin/project-api .

# Run the service.
run:
	go run .

# Run the service with debug logging.
debug:
	go run . -d

# Run the unit tests.
test:
	go test .
