# Claude Development Guide for LFX V2 Project Service

This guide provides essential information for Claude instances working with the LFX V2 Project Service codebase. It includes build commands, architecture patterns, and key technical decisions.

## Project Overview

The LFX V2 Project Service is a RESTful API service that manages projects within the Linux Foundation's LFX platform. It provides CRUD operations for projects with built-in authorization and audit capabilities.

### Key Technologies

- **Language**: Go 1.23+
- **API Framework**: Goa v3 (code generation framework)
- **Messaging**: NATS with JetStream for event-driven architecture
- **Storage**: NATS Key-Value stores (no traditional database)
- **Authentication**: JWT with Heimdall middleware
- **Authorization**: OpenFGA for fine-grained access control
- **Container**: Chainguard distroless images
- **Orchestration**: Kubernetes with Helm charts

## Architecture Overview

The service follows **Clean Architecture** principles with clear separation of concerns:

```text
.github/                    # CI/CD workflow files for Github Actions

charts/                     # Helm charts containing kubernetes template files for deployments

cmd/project-api/            # Presentation Layer (HTTP/NATS handlers)
├── design/                 # Goa API design specifications
├── gen/                    # Generated code (not committed)
├── service*.go            # HTTP and NATS handlers

internal/                   # Core business logic
├── domain/                # Domain layer (interfaces, models, errors)
│   └── models/           # Domain entities
├── service/              # Service layer (business logic)
├── infrastructure/       # Infrastructure layer
│   ├── auth/            # JWT authentication
│   └── nats/            # NATS repository implementation
└── middleware/          # HTTP middleware

pkg/                    # Shared packages across services
└── constants/          # Shared constants

scripts/                # Scripts for services and miscellaneous tasks
```

### Key Design Principles

1. **Database Independence**: Repository interfaces allow switching storage backends
2. **Testability**: Each layer can be tested in isolation using mocks
3. **Event-Driven**: All data changes trigger NATS messages for downstream services
4. **Separation of Concerns**: Clear boundaries between layers

## Development Workflow

### Prerequisites

```bash
# Install Go 1.23+
# Install Goa framework
go install goa.design/goa/v3/cmd/goa@latest

# Install linting tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Common Development Tasks

#### 1. Generate API Code (REQUIRED after design changes)

```bash
cd cmd/project-api
make apigen
# or directly: goa gen github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/design
```

#### 2. Build the Service

```bash
cd cmd/project-api
make build
```

#### 3. Run Tests

```bash
cd cmd/project-api
make test              # Run unit tests
make test-verbose      # Verbose output
make test-coverage     # Generate coverage report
make test-integration  # Run integration tests (requires -tags=integration)
```

#### 4. Run the Service Locally

```bash
cd cmd/project-api
# Basic run
make run

# With debug logging
make debug

# With custom flags
go run . -d -p 8080
```

#### 5. Lint and Format Code

```bash
cd cmd/project-api
make fmt    # Format code
make lint   # Run golangci-lint
make check  # Check format and lint without modifying
```

## Code Generation (Goa Framework)

The service uses Goa v3 for API code generation. This is **critical** to understand:

1. **Design First**: API is defined in `cmd/project-api/design/` files
2. **Generated Code**: Running `make apigen` generates:
   - HTTP server/client code
   - Service interfaces
   - OpenAPI specifications
   - Type definitions
3. **Implementation**: You implement the generated interfaces in `service*.go` files

### Adding New Endpoints

1. Update `design/project.go` with new method
2. Run `make apigen` to regenerate code
3. Implement the new method in `service_endpoint_project.go`
4. Add tests in `service_endpoint_project_test.go`
5. Update Heimdall ruleset in `charts/*/templates/ruleset.yaml`

## NATS Messaging Patterns

The service uses NATS for:

1. **Storage**: Key-Value stores for project data
2. **Events**: Publishing events on data changes
3. **RPC**: Handling requests from other services

### Key-Value Stores

- `projects`: Base project information
- `project-settings`: Project settings (separated for access control)

### Message Subjects

```go
// Outbound events (published by this service)
"lfx.index.project"                    // Project created/updated
"lfx.index.project_settings"           // Settings updated
"lfx.update_access.project"            // Access control updates
"lfx.delete_all_access.project"        // Access control deletion

// Inbound RPC (handled by this service)
"lfx.projects-api.get_name"            // Get project name by UID
"lfx.projects-api.get_slug"            // Get project slug by UID
"lfx.projects-api.slug_to_uid"         // Convert slug to UID
```

## Testing Patterns

### Unit Tests

- Mock all external dependencies (repository, message builder)
- Test each layer in isolation
- Use table-driven tests for comprehensive coverage
- Write one function tests containing multiple test cases that focus on a single function
- Focus on testing exported functions of packages
- Unit tests should be alongside the implementation code with the same file name with a suffix of `*_test.go`
- **IMPORTANT**: Each function should have exactly ONE corresponding test function (e.g., `SendIndexProject` → `TestMessageBuilder_SendIndexProject`) which can have multiple tests cases within it.
- Add test cases within existing test functions if the function you are trying to test already has one rather than creating new test functions

### Example Test Structure

```go
func TestEndpoint(t *testing.T) {
    tests := []struct {
        name       string
        payload    *projsvc.Payload
        setupMocks func(*domain.MockRepo, *domain.MockMsg)
        wantErr    bool
    }{
        // Test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            api, mockRepo, mockMsg := setupAPI()
            tt.setupMocks(mockRepo, mockMsg)
            // Test logic
        })
    }
}
```

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP listen port | 8080 | No |
| `NATS_URL` | NATS server URL | nats://localhost:4222 | No |
| `LOG_LEVEL` | Log level | info | No |
| `JWKS_URL` | JWT verification endpoint | - | No |
| `AUDIENCE` | JWT audience | lfx-v2-project-service | No |
| `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL` | Mock auth for local dev | - | No |

## Authorization (OpenFGA)

When deployed, the service uses OpenFGA for authorization:

- **GET /projects** - No check (public list)
- **POST /projects** - Requires `writer` on parent (if specified)
- **GET /projects/:id** - Requires `viewer` on project
- **GET /projects/:id/settings** - Requires `auditor` on project
- **PUT /projects/:id** - Requires `writer` on project
- **PUT /projects/:id/settings** - Requires `writer` on project
- **DELETE /projects/:id** - Requires `owner` on project

## Local Development Setup

### 1. Start NATS with JetStream

```bash
# Using Docker
docker run -p 4222:4222 nats:latest -js

# Create KV stores
nats kv add projects --history=20 --storage=file
nats kv add project-settings --history=20 --storage=file
```

### 2. Run the Service

```bash
cd cmd/project-api
export NATS_URL=nats://localhost:4222
export JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=test-user
make run
```

### 3. Test the API

```bash
# Health check
curl http://localhost:8080/livez

# Get projects (requires auth header in production)
curl http://localhost:8080/projects?v=1
```

## Docker Build

```bash
# Build from repository root
docker build -t lfx-v2-project-service:latest .

# The Dockerfile uses:
# - Chainguard Go image for building
# - Chainguard static image for runtime (distroless)
# - Multi-stage build for minimal image size
```

## Kubernetes Deployment

```bash
# Install Helm chart
helm install lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx

# Update deployment
helm upgrade lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx

# View generated manifests
helm template lfx-v2-project-service ./charts/lfx-v2-project-service/ -n lfx
```

### Helm Configuration

- OpenFGA can be disabled for local development
- NATS KV buckets are created automatically
- Heimdall middleware handles JWT validation
- Traefik IngressRoute for HTTP routing

## CI/CD Pipeline

GitHub Actions workflows:

- **mega-linter.yml**: Comprehensive linting (Go, YAML, Docker, etc.)
- **project-api-build.yml**: Build and test on PRs
- **license-header-check.yml**: Ensure proper licensing

### PR Checks

1. Generate API code
2. Build binary
3. Run unit tests
4. Lint with MegaLinter

## Common Pitfalls and Solutions

### 1. Forgetting to Generate Code

**Problem**: Changes to design files not reflected in implementation
**Solution**: Always run `make apigen` after modifying design files

### 2. ETag Handling

**Problem**: Concurrent updates without proper ETag validation
**Solution**: Always include ETag header in PUT/DELETE requests

### 3. NATS Connection

**Problem**: Service fails to start due to NATS connection
**Solution**: Ensure NATS is running and NATS_URL is correct

### 4. Slug Validation

**Problem**: Invalid slug format causes API errors
**Solution**: Slugs must match `^[a-z][a-z0-9_\-]*[a-z0-9]$`

### 5. Parent Project Validation

**Problem**: Creating projects with invalid parent_uid
**Solution**: parent_uid must be empty string or valid UUID

## Mock Data Loading

Use the provided script to load test data:

```bash
cd scripts/load_mock_data
go run main.go -bearer-token "your-token" -num-projects 10
```

## Key Implementation Details

### 1. Project Data Split

Projects are split into two parts for access control:

- **Base**: Core project info (stored in `projects` KV)
- **Settings**: Sensitive settings (stored in `project-settings` KV)

### 2. Message Publishing

Every data modification publishes NATS messages:

- Index messages for search service
- Access control updates for authorization service

### 3. Request Context

Important context values:

- `request-id`: Unique request identifier
- `authorization`: JWT token from header
- `etag`: ETag for optimistic concurrency

### 4. Error Handling

Domain errors are mapped to HTTP status codes:

- `ErrNotFound` → 404
- `ErrConflict` → 409
- `ErrInvalidParentUID` → 400
- `ErrInternal` → 500

## Debugging Tips

1. **Enable Debug Logging**: Run with `-d` flag or set `LOG_LEVEL=debug`
2. **Check NATS Messages**: Use `nats sub "lfx.>"` to monitor all messages
3. **Verify KV Data**: Use `nats kv get projects <uid>` to check stored data
4. **HTTP Traces**: Middleware logs all requests with timing
5. **Generated Code**: Check `gen/` directory for Goa-generated interfaces

## Contributing Guidelines

1. **Design First**: Update Goa design files before implementation
2. **Test Coverage**: Write comprehensive unit tests
3. **Mock External Deps**: Use mocks for repository and message builder
4. **Follow Clean Architecture**: Respect layer boundaries
5. **Update Docs**: Keep README files current
6. **Lint Clean**: Ensure `make check` passes

## Resources

- [Goa Framework Docs](https://goa.design/docs/)
- [NATS JetStream Docs](https://docs.nats.io/jetstream)
- [OpenFGA Docs](https://openfga.dev/docs)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
