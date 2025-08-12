# Development Guide

This guide provides comprehensive instructions for developers working on the LFX V2 Project Service, including building, testing, and deploying the service locally.

## Prerequisites

Before you begin development, ensure you have the following tools installed:

- **Go 1.23+** - [Download and install Go](https://golang.org/dl/)
- **Goa v3** - Code generation framework

  ```bash
  go install goa.design/goa/v3/cmd/goa@latest
  ```

- **golangci-lint** - Linting tool

  ```bash
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  ```

- **Docker** - For building container images
- **Kubernetes** - For local deployment (optional)
- **Helm** - For deploying to Kubernetes (optional)

## Development Setup Options

You have two main options for setting up your development environment:

### Option A: Full Platform Setup (Recommended for Integration Testing)

If you need to test with the full LFX platform stack (including Heimdall, OpenFGA, Authelia, Traefik), first install the lfx-platform chart:

```bash
# Create namespace
kubectl create namespace lfx

# Install lfx-platform chart (includes all dependencies)
helm install -n lfx lfx-platform \
  oci://ghcr.io/linuxfoundation/lfx-v2-helm/chart/lfx-platform \
  --version 0.1.12
```

This provides:

- NATS with JetStream (messaging and storage)
- Heimdall (authorization middleware)
- OpenFGA (fine-grained access control)
- Authelia (authentication)
- Traefik (HTTP routing)

Then follow the [Helm Local Development](#helm-local-development) section below.

### Option B: Minimal Setup (Recommended for Quick Development)

For rapid development and testing without the full platform:

```bash
# Just run NATS locally
docker run -d -p 4222:4222 nats:latest -js
```

**Insert Root Project**

All projects that are created require a `parent_uid` to be set. This means that if there are no existing projects, it is impossible to create a new project. To resolve this, you can insert a root project when developing locally using [the root_project_setup tooling](./scripts/root-project-setup/README.md).

This will create a root project with a randomly generated UID and a slug of `ROOT` which is expected to be used as the root of the project hierarchy.

The [helm chart](./charts/lfx-v2-project-service) runs this automatically as an init container.

This is sufficient for:

- Local development
- Unit testing
- API endpoint testing (with mock auth)

Note: this means that there is no authentication/authorization with heimdall. So only do this if you are developing the service without a need for testing the full flow of authentication.

Then follow the [Local Development Without Kubernetes](#local-development-without-kubernetes) section below.

## Development Workflow

### 1. Code Generation

This service uses [Goa v3](https://goa.design/) for API code generation. The API design is defined in `api/project/v1/design/` and code is generated to `api/project/v1/gen/`.

**Important:** Always regenerate code after modifying design files!

```bash
# Generate API code from design files
make apigen

# This runs: goa gen github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/design -o api/project/v1
```

The generated code includes:

- HTTP server and client code
- Service interfaces
- OpenAPI specifications
- Type definitions

### 2. Building the Service

```bash
# Build the binary
make build

# This creates: bin/project-api
```

### 3. Running the Service

#### Environment Variables

Configure the service using the environment variables from [.env.example](.env.example) and any modifications needed in a copied `.env` file:

```bash
cp .env.example .env

source .env
```

#### Option A: Direct Go Run

```bash
# Run with default settings
make run

# Run with debug logging
make debug

# Run with custom flags
go run ./cmd/project-api -d -p 8080
```

#### Option B: Using the Built Binary

```bash
# Build first
make build

# Run the binary
./bin/project-api -d
```

### 4. Linting and Formatting

```bash
# Format code
make fmt

# Run linter
make lint

# Check formatting and linting without modifying files
make check
```

### 5. Testing

```bash
# Run unit tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage report
make test-coverage

# Run specific test
go test -v ./cmd/project-api -run TestCreateProject
```

### 6. Clean Build Artifacts

```bash
# Remove generated files and binaries
make clean
```

## Making Code Changes

### Typical Development Cycle

1. **Modify API Design** (if changing the API)

   ```bash
   # Edit files in api/project/v1/design/
   vim api/project/v1/design/project.go
   ```

2. **Generate Code**

   ```bash
   make apigen
   ```

3. **Implement Changes**

   ```bash
   # Implement service endpoints
   vim cmd/project-api/service_endpoint_project.go
   
   # Add/modify business logic
   vim internal/service/project_service.go
   ```

4. **Test Your Changes**

   ```bash
   # Run tests
   make test
   
   # Check linting
   make check
   ```

5. **Run Locally**

   ```bash
   # Run the service to make API requests to the server to check behavior
   make debug
   ```

6. **Test the API**

   ```bash
   # Health check
   curl http://localhost:8080/livez
   
   # List projects
   curl http://localhost:8080/projects
   ```

## Docker Development

### Building the Docker Image

```bash
# Option 1: Using make command
make docker-build

# Option 2: Using docker directly
docker build -t linuxfoundation/lfx-v2-project-service:latest .

# The Dockerfile uses:
# - Multi-stage build for minimal image size
# - Chainguard Go image for building
# - Chainguard static image for runtime (distroless)
```

### Running with Docker

```bash
# Run NATS first
docker run -d --name nats -p 4222:4222 nats:latest -js

# Run the service
docker run -d \
  --name project-service \
  -p 8080:8080 \
  -e NATS_URL=nats://host.docker.internal:4222 \
  -e JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=test-user \
  lfx-v2-project-service:latest

# Check logs
docker logs project-service

# Test the service
curl http://localhost:8080/livez
```

## Helm Local Development

### Using Local Values File

For local development with Helm, you'll need to create a custom values file:

1. **Create a local values file**

   ```bash
   # Copy the default values as a starting point
   cp charts/lfx-v2-project-service/values.yaml charts/lfx-v2-project-service/values.local.yaml
   ```

2. **Modify values.local.yaml for local development**

   ```yaml
   # Edit charts/lfx-v2-project-service/values.local.yaml to use the local docker image repository
   image:
     repository: linuxfoundation/lfx-v2-project-service
     tag: latest
     pullPolicy: Never  # Don't try to pull from registry
   
   # Add any other local overrides
   ```

### Building and Deploying Locally

1. **Build Local Docker Image**

   ```bash
   # Build with a local tag (matching the repository in values.local.yaml)
   docker build -t linuxfoundation/lfx-v2-project-service:latest .
   
   # Optional: If using minikube, load the image
   minikube image load linuxfoundation/lfx-v2-project-service:latest
   
   # Optional: If using kind, load the image
   kind load docker-image linuxfoundation/lfx-v2-project-service:latest
   ```

2. **Install/Upgrade Helm Chart with Local Values**

   ```bash
   make helm-install-local
   ```

3. **Verify Deployment**

   ```bash
   # Check pod status
   kubectl get pods -n lfx | grep project-service
   
   # Check logs
   kubectl logs -n lfx -l app=lfx-v2-project-service
   
   # Port forward for testing
   kubectl port-forward -n lfx svc/lfx-v2-project-service 8080:80
   
   # Test the service
   curl http://localhost:8080/projects
   ```

### Quick Development Iteration

For rapid development iteration:

```bash
# 1. Make your code changes

# 2. Build and load new image
make docker-build

# 3. Restart the pod to use new image
make helm-restart

# 4. Watch the logs
kubectl logs -f -n lfx -l app=lfx-v2-project-service
```

## Local Development Without Kubernetes

For lightweight local development without Kubernetes:

NOTE: When running the service directly (outside of Kubernetes/Helm), you're bypassing:

- **Traefik** load balancer and routing
- **Heimdall** middleware for authentication and authorization
- **OpenFGA** fine-grained access control

This means:

- All API requests will bypass authentication checks
- The `JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL` environment variable simulates a specific logged-in user

1. **Start NATS with JetStream**

   ```bash
   docker run -d -p 4222:4222 nats:latest -js
   ```

2. **Create Required KV Stores**

   ```bash
   # Install NATS CLI
   go install github.com/nats-io/natscli/nats@latest
   
   # Create KV stores
   nats kv add projects --history=20 --storage=file
   nats kv add project-settings --history=20 --storage=file
   ```

3. **Run the Service**

   ```bash
   # NATS server URL
   export NATS_URL=nats://localhost:4222
   
   # Mock authentication - simulates a logged-in user (bypasses real auth)
   export JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=test-user
   
   # Enable debug logging
   export LOG_LEVEL=debug
   
   make run
   ```

4. **Test API Endpoints**

   ```bash
   # Create a project (no auth header needed with mock auth)
   curl -X POST http://localhost:8080/projects \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Test Project",
       "slug": "test-project",
       "description": "A test project"
     }'
   
   # List projects (no auth header needed with mock auth)
   curl http://localhost:8080/projects
   ```

## Authorization with OpenFGA

When deployed via Kubernetes, the project service uses OpenFGA for fine-grained authorization control. The authorization is handled by Heimdall middleware before requests reach the service.

### Configuration

OpenFGA authorization is controlled by the `openfga.enabled` value in the Helm chart:

```yaml
# In values.yaml or via --set flag
openfga:
  enabled: true  # Enable OpenFGA authorization (default)
  # enabled: false  # Disable for local development only
```

### Authorization Rules

When OpenFGA is enabled, the following authorization checks are enforced:

- **GET /projects** - No OpenFGA check (returns list of all projects)
- **POST /projects** - Requires `writer` relation on the parent project (if parent_uid is specified)
- **GET /projects/:id** - Requires `viewer` relation on the specific project
- **GET /projects/:id/settings** - Requires `auditor` relation on the specific project
- **PUT /projects/:id** - Requires `writer` relation on the specific project
- **PUT /projects/:id/settings** - Requires `writer` relation on the specific project
- **DELETE /projects/:id** - Requires `owner` relation on the specific project

### Local Development

For local development without OpenFGA:

1. Set `openfga.enabled: false` in your Helm values
2. All requests will be allowed through (after JWT authentication)
3. **Warning**: Never disable OpenFGA in production environments

## Architecture Overview

The service follows Clean Architecture principles. For the complete file structure, see the [File Structure section in README.md](README.md#file-structure).

## Common Development Tasks

### Adding a New Endpoint

1. **Update API Design**

   ```go
   // api/project/v1/design/project.go
   Method("NewEndpoint", func() {
       Description("New endpoint description")
       Payload(NewEndpointPayload)
       Result(NewEndpointResult)
       HTTP(func() {
           POST("/projects/{id}/action")
           Response(StatusOK)
       })
   })
   ```

2. **Generate Code**

   ```bash
   make apigen
   ```

3. **Implement Endpoint**

   ```go
   // cmd/project-api/service_endpoint_project.go
   func (s *projectsrvc) NewEndpoint(ctx context.Context, p *projsvc.NewEndpointPayload) (*projsvc.NewEndpointResult, error) {
       // Implementation
   }
   ```

4. **Add Tests**

   ```go
   // cmd/project-api/service_endpoint_project_test.go
   func TestNewEndpoint(t *testing.T) {
       // Test implementation
   }
   ```

5. **Update Heimdall Rules** (if needed)

   ```yaml
   # charts/lfx-v2-project-service/templates/ruleset.yaml
   ```

### Debugging Tips

1. **Enable Debug Logging**

   ```bash
   export LOG_LEVEL=debug
   make run
   ```

2. **Monitor NATS Messages**

   ```bash
   # Subscribe to all LFX projects-api messages
   nats sub "lfx.projects-api.*"
   ```

3. **Check KV Store Data**

   ```bash
   # Get project data
   nats kv get projects <uid>
   
   # List all keys
   nats kv ls projects
   ```

4. **Use Request Tracing**

   ```bash
   # Send request with trace header
   curl -H "X-Request-ID: test-123" http://localhost:8080/projects
   ```

## Troubleshooting

### Common Issues

1. **"goa: command not found"**

   ```bash
   go install goa.design/goa/v3/cmd/goa@latest
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

2. **NATS Connection Failed**
   - Ensure NATS is running: `docker ps | grep nats`
   - Check NATS_URL environment variable
   - Verify network connectivity

3. **Generated Code Out of Sync**
   - Always run `make apigen` after design changes
   - Check for uncommitted generated files

4. **Tests Failing**
   - Run `make clean && make apigen` to regenerate code
   - Check for missing mock expectations

## Additional Resources

- [Goa Framework Documentation](https://goa.design/docs/)
- [NATS Documentation](https://docs.nats.io/)
- [Project README](README.md) - Quick start guide
- [CLAUDE.md](CLAUDE.md) - AI assistant instructions and detailed technical documentation
