# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
Go REST API service for managing LFX projects using Goa framework for design-first API generation, NATS JetStream for messaging and storage, and Kubernetes Gateway API for routing.

## Architecture

### Core Components
- **API Framework**: Goa v3 (design-first code generation from DSL)
- **Storage**: NATS JetStream Key-Value store (`projects` bucket)
- **Messaging**: NATS pub/sub for inter-service communication
- **Authentication**: JWT via Heimdall middleware with OpenFGA authorization
- **Routing**: Kubernetes Gateway API (HTTPRoute) attached to platform Gateway
- **Deployment**: Kubernetes with Helm charts

### Project Structure
```
cmd/project-api/          # Main service implementation
├── design/              # Goa DSL API definitions (source of truth)
├── gen/                 # Auto-generated code (never edit manually)
├── service*.go          # Endpoint implementations
├── main.go              # Entry point with NATS/HTTP setup
└── Makefile             # Build and development commands

internal/                # Private application code
├── infrastructure/nats/ # NATS abstractions and models
├── middleware/          # HTTP middleware (auth, logging, request ID)
└── log/                # Structured logging configuration

pkg/constants/           # Shared constants and environment configs
charts/                  # Kubernetes Helm deployment
├── templates/httproute.yaml  # Gateway API routing
└── values.yaml          # Configuration
```

### Data Flow Architecture
1. **HTTP Requests** → Gateway API HTTPRoute → Service
2. **Authentication** → Heimdall JWT validation → OpenFGA authorization
3. **Data Storage** → NATS KV store with slug-based indexing
4. **Messaging** → NATS subjects for project queries and updates
5. **Project Hierarchy** → All projects require `parent_uid` (except ROOT)

## Development Commands

### Essential Workflow (run in `cmd/project-api/`)
```bash
make deps           # Install dependencies (goa CLI, golangci-lint)
make apigen         # Regenerate API code from design/ (REQUIRED after design changes)
make build          # Build binary to bin/project-api
make run            # Run service (auto-generates API first)
make debug          # Run with debug logging (-d flag)
```

### Testing
```bash
make test           # Unit tests with race detection
make test-verbose   # Verbose test output  
make test-coverage  # Generate coverage report (coverage/coverage.html)
make test-integration # Integration tests (requires -tags=integration)
make test-authorization # Run authorization test script
```

### Code Quality
```bash
make fmt            # Format code (gofmt + gofmt -s)
make lint           # Run golangci-lint
make check          # Verify formatting + linting (CI-friendly)
make verify         # Ensure generated code is up-to-date
```

### Development Workflow
```bash
make clean          # Remove bin/, coverage/, go clean cache
make all            # Full pipeline: clean, deps, apigen, fmt, lint, test, build
```

## Key Architectural Concepts

### Goa Framework Integration
- **Design-First**: API defined in `design/project.go` using Goa DSL
- **Code Generation**: `make apigen` generates HTTP handlers, OpenAPI specs, client code
- **Never Edit gen/**: All files in `gen/` are auto-generated and will be overwritten
- **Service Implementation**: Implement interfaces in `service_endpoint_*.go` files

### NATS Storage Patterns
- **KV Store**: `projects` bucket stores project JSON by UID
- **Indexing**: `slug/{slug}` keys map slugs to UIDs for lookups
- **Messaging**: Queue subscriptions for project name/slug resolution
- **Environment Prefixes**: Subject names include environment (`dev.`, `stg.`, `prod.`)

### Authentication & Authorization Flow
1. **HTTPRoute** → Heimdall middleware (ExtensionRef filter)
2. **Heimdall** → JWT validation + principal extraction  
3. **OpenFGA** → Fine-grained authorization based on project relationships
4. **Service** → Receives validated JWT with principal context

### Gateway API Configuration
- **HTTPRoute**: Two separate rules for auth vs non-auth endpoints
  - `/projects*` endpoints: WITH Heimdall authentication
  - `/livez`, `/readyz` endpoints: NO authentication (health checks)
- **Platform Integration**: Attaches to `lfx-platform-gateway` in `lfx` namespace
- **Values Configuration**: `gatewayAPI.parentGateway` configures platform Gateway reference

## Common Development Tasks

### Adding New API Endpoints
1. Update `design/project.go` with new method definition
2. Run `make apigen` to generate interfaces and HTTP handlers
3. Implement interface methods in appropriate `service_endpoint_*.go` file
4. Add authorization rules to Helm chart `ruleset.yaml` if needed
5. Update tests

### Modifying Existing Endpoints
1. Update method signature in `design/project.go`
2. Run `make apigen` to regenerate code
3. Update implementation in `service_endpoint_*.go`
4. Update tests to match new signature

### NATS Message Handlers
1. Define handler method in `service_handler.go`
2. Register subscription in `createNatsSubcriptions()` function in `main.go`
3. Add message types to `internal/infrastructure/nats/` if needed
4. Update subject constants in `pkg/constants/nats.go`

### Environment Configuration
- **Environment Variables**: Parsed in `parseEnv()` function in `main.go`
- **Common Variables**: `PORT`, `NATS_URL`, `LFX_ENVIRONMENT`, `JWKS_URL`, `AUDIENCE`
- **Local Development**: Set `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=true` to bypass auth

### Helm Chart Updates
- **Configuration**: Update `charts/lfx-v2-project-service/values.yaml`
- **Templates**: Modify Kubernetes resources in `charts/lfx-v2-project-service/templates/`
- **Testing**: `make helm-templates` to preview rendered templates
- **Gateway API**: HTTPRoute automatically attaches to platform Gateway

## Project-Specific Patterns

### Error Handling
- Use `createResponse(statusCode, message)` helper for consistent HTTP errors
- Standard error types: BadRequest, NotFound, Conflict, InternalServerError, ServiceUnavailable
- NATS errors should be logged with context and return ServiceUnavailable to client

### Project Data Model
```go
type ProjectDB struct {
    UID         string    `json:"uid"`         // UUID primary key
    Slug        string    `json:"slug"`        // URL-safe identifier  
    Name        string    `json:"name"`        // Display name
    ParentUID   string    `json:"parent_uid"`  // Required (except ROOT project)
    Public      bool      `json:"public"`      // Visibility flag
    Auditors    []string  `json:"auditors"`    // Read-only access list
    Writers     []string  `json:"writers"`     // Write access list
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### NATS Key Patterns
- **Project by UID**: `{uid}` → full project JSON
- **Slug to UID lookup**: `slug/{slug}` → UID string
- **Message Subjects**: `{env}.lfx.index.project`, `{env}.lfx.update_access.project`

### Testing Patterns
- **Unit Tests**: Focus on business logic in `service_endpoint_*_test.go`
- **NATS Mocking**: Use interfaces in `internal/infrastructure/nats/`
- **Authorization Tests**: Integration tests verify OpenFGA rules
- **HTTP Tests**: Test generated Goa HTTP handlers

## Deployment

### Local Development
1. Ensure NATS server with JetStream is running
2. Create ROOT project via `scripts/root-project-setup/`
3. Set environment variables for local config
4. Run `make run` or `make debug`
