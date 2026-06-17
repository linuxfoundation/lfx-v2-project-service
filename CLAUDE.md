# Claude Development Guide for LFX V2 Project Service

This guide provides essential information for Claude instances working with the LFX V2 Project Service codebase. It includes build commands, architecture patterns, and key technical decisions.

> **Central LFX skills:**
> - `/lfx-skills:lfx`: cross-repo routing, "where does X live" questions, owner/peer repos, missing checkouts.
> - `/lfx-skills:lfx-platform-architecture`: platform composition, V2 service classes (native, wrapper, proxy, platform), write/read/access-check flows, cross-service responsibilities, NATS/KV ownership, handoff points across Self Serve, FGA, indexer, query, Heimdall, OpenFGA, Helm, ArgoCD.
>
> **Repo-local project-service skills and docs:**
> - `/project-service-dev` at `.claude/skills/project-service-dev/` auto-attaches on Go and service paths and owns logging, errors, request context, pagination, generated-code boundary, NATS/KV publishing, tests, formatting, linting, and license headers for this repo.
> - `/project-service-pr-readiness` checks pre-PR shape only: branch/JIRA/conventional commits/rebase/DCO+GPG/diff size/protected files.
> - `/project-service-preflight` runs the mechanical Go pre-PR pipeline after readiness: working tree, license, formatting, lint, build, tests, protected files, commit verification, generated-code freshness, and change summary.
> - Repo-local docs under `docs/` own concrete subjects, payloads, emitted contracts, and domain behavior; this repo's chart owns project-service Helm values and templates.
> - If the central plugin is missing, install with `/plugin marketplace add linuxfoundation/lfx-skills` then `/plugin install lfx-skills@lfx-skills`.

## Project Overview

The LFX V2 Project Service is a RESTful API service that manages projects within the Linux Foundation's LFX platform. It provides CRUD operations for projects with built-in authorization and audit capabilities.

### Key Technologies

- **Language**: Go 1.25+
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

api/                        # API contracts
└── project/
    └── v1/
        ├── design/         # Goa API design specifications
        └── gen/            # Generated code (gitignored)

charts/                     # Helm charts containing kubernetes template files for deployments

cmd/project-api/            # Presentation Layer (HTTP entry point, Goa endpoint adapters)
├── service_endpoint_*.go  # Goa endpoint adapters (project, link, folder, document)
├── http.go                # HTTP server wiring
└── main.go                # Application entry point, NATS subscription wiring

internal/                   # Core business logic
├── domain/                # Domain layer (interfaces, models, errors, mocks)
│   └── models/           # Domain entities (project, link, folder, document)
├── service/               # Service layer (business logic, NATS RPC handlers, event subscriber)
│   ├── *_operations.go   # Per-resource business orchestration
│   ├── project_handlers.go    # Inbound NATS request/reply RPC handlers
│   ├── project_subscriber.go  # Inbound NATS event subscribers (settings updates, invite acceptance)
│   ├── document_subscriber.go # Inbound NATS event subscribers (document/link created notifications)
│   ├── converters.go     # Domain ↔ Goa ↔ pkg/events wire-type converters
│   └── email/            # Email template rendering (one file per email type)
└── infrastructure/        # Infrastructure layer
    ├── auth/             # JWT authentication
    ├── log/              # Structured logging helpers (AppendCtx, InitStructureLogConfig)
    ├── middleware/        # HTTP middleware (auth, request ID, body limit, logger)
    └── nats/             # NATS repository, object store, message builder, user reader

pkg/                    # Shared packages across services
├── constants/          # Shared constants (NATS subjects, KV buckets, HTTP, access control)
└── events/             # NATS event wire types consumed by other services

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
# Install Go 1.25+
# Install Goa framework
go install goa.design/goa/v3/cmd/goa@v3.22.6

# Install linting tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Common Development Tasks

#### 1. Generate API Code (REQUIRED after design changes)

```bash
make apigen
# or directly: goa gen github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/design -o api/project/v1
```

#### 2. Build the Service

```bash
make build
```

#### 3. Run Tests

```bash
make test              # Run unit tests
make test-verbose      # Verbose output
make test-coverage     # Generate coverage report
```

#### 4. Run the Service Locally

```bash
# Basic run
make run

# With debug logging
make debug

# With custom flags (direct go run)
go run ./cmd/project-api -d -p 8080
```

#### 5. Lint and Format Code

```bash
make fmt    # Format code
make lint   # Run golangci-lint
make check  # Check format and lint without modifying
```

## Work cycle — post-commit and pre-PR reviews

> **CRITICAL — while the branch is pre-PR, post-commit review is mandatory.** After every commit on the local branch, launch all three reviewer subagents via the Agent tool in parallel: `lfx-skills:lfx-general-code-reviewer`, `lfx-skills:lfx-project-service-code-reviewer`, AND `lfx-skills:lfx-project-service-learnings-reviewer` (each with `run_in_background: true`) — then keep working while they run. If Claude displays plugin agents without the `lfx-skills:` namespace, use the equivalent displayed general, project-service, and learnings reviewer names. Before opening a PR, every running review must return clean (or remaining findings explicitly documented as trade-offs), the **full-branch sweep** must run clean if the branch has more than one commit (`branch` arg), AND `/project-service-pr-readiness` must clear every Critical finding before `/project-service-preflight` runs.
>
> **Once the PR is open, do NOT invoke these reviewers on iteration commits.** CodeRabbit + Copilot auto-trigger on every push and own the audit surface from that point. The general, project-service, and learnings reviewers are pre-PR insurance only.

### Post-commit (pre-PR phase, after every commit, asynchronous)

1. **Commit your work.** `git commit -s -S`. Do not wait for any prior review to finish.
2. **Immediately launch all three reviewer subagents in parallel.** Use `subagent_type: lfx-skills:lfx-general-code-reviewer`, `subagent_type: lfx-skills:lfx-project-service-code-reviewer`, and `subagent_type: lfx-skills:lfx-project-service-learnings-reviewer`, each with `run_in_background: true`.
3. **Post-commit mode prompt for all three reviewers (exact):** `target repo: lfx-v2-project-service\n\nReview the latest commit.` Append `extra: <focus>` on a new line only when there is a priority hint to add. Do NOT pass `branch` here. If this work cycle is launched from the LFX workspace parent, the `target repo:` line is required so all three reviewers operate in this repo.
4. **Keep working.** Start the next commit while the reviewers run. Do not block on them.
5. **When reviews return:** roll every Critical finding and every reasonable Important finding into the next commit.

### Pre-PR (drain the queue, sweep cumulative state, then open)

When the work is done and no more code commits are planned:

1. **Wait for every running review to complete.**
2. **If any returned review flags Critical or reasonable Important:** add a fix commit, launch all three reviewers again on the new state, wait, and loop until clean or explicitly documented as a trade-off.
3. **Full-branch sweep — only if the branch has more than one commit.** Launch `lfx-skills:lfx-general-code-reviewer`, `lfx-skills:lfx-project-service-code-reviewer`, and `lfx-skills:lfx-project-service-learnings-reviewer` again with prompt **`target repo: lfx-v2-project-service\nbranch\n\nReview the branch's diff against origin/main.`**. Address any new findings, then re-run the sweep until clean.
4. **Run `/project-service-pr-readiness`** for branch and commit shape only.
5. **Run `/project-service-preflight`** for mechanical Go validation and PR summary.
6. **Only then push and open the PR.**

### Post-PR iteration (responding to bot feedback on an open PR)

1. Wait for CodeRabbit + Copilot to comment after each push.
2. Triage every Critical and reasonable Important finding against current code.
3. Roll fixes into a `fix(review): ...` commit.
4. Push. Repeat until clean.

## Code Generation (Goa Framework)

The service uses Goa v3 for API code generation. This is **critical** to understand:

1. **Design First**: API is defined in `api/project/v1/design/` files
2. **Generated Code**: Running `make apigen` generates to `api/project/v1/gen/`:
   - HTTP server/client code
   - Service interfaces
   - OpenAPI specifications
   - Type definitions
3. **Implementation**: You implement the generated interfaces in `cmd/project-api/service*.go` files

### Adding New Endpoints

1. Update `api/project/v1/design/project.go` with new method
2. Run `make apigen` (from repository root) to regenerate code
3. Implement the Goa endpoint adapter in `cmd/project-api/service_endpoint_*.go` (translation only); put business logic in `internal/service/*_operations.go`
4. Add tests alongside the implementation (`internal/service/*_operations_test.go` and the adapter test)
5. Update Heimdall ruleset in `charts/*/templates/ruleset.yaml`

## NATS Messaging Patterns

The service uses NATS for:

1. **Storage**: Key-Value stores for project data
2. **Events**: Publishing events on data changes
3. **RPC**: Handling requests from other services

### Key-Value Stores

- `projects`: Base project information
- `project-settings`: Project settings (separated for access control)
- `project-links`: Project link records
- `project-folders`: Project folder records
- `project-documents-metadata`: Project document metadata
- `project-documents`: Project document binaries (NATS object store)

All bucket names live as constants in `pkg/constants/nats.go`.

### API Endpoints and Message Subjects

Complete API endpoint documentation and NATS message handlers are now documented in README.md.

There are two distinct NATS patterns in this service — both use `QueueSubscribe` but for different purposes:

**Request/reply RPC** (`internal/service/project_handlers.go`): another service sends a request and blocks waiting for a response. The handler calls `msg.Respond(data)` to return data to the caller.

**Event subscriptions** (`internal/service/project_subscriber.go` and `internal/service/document_subscriber.go`): the service reacts to events that were already published (including by itself). No caller is waiting — the handler is fire-and-forget and never calls `msg.Respond`.

```go
// Inbound RPC — request/reply, caller blocks waiting for response
"lfx.projects-api.get_name"            // Get project name by UID
"lfx.projects-api.get_slug"            // Get project slug by UID
"lfx.projects-api.get_logo"            // Get project logo URL by UID
"lfx.projects-api.slug_to_uid"         // Convert slug to UID
"lfx.projects-api.get_parent_uid"      // Get parent project UID

// Inbound events — fire-and-forget, no reply expected
"lfx.projects-api.project_settings.updated" // Self-published; sends role notification emails / invites on member changes
"lfx.invite-service.invite_accepted"   // From invite-service (enriched event); promotes matching email-only users to LFID across all projects
"lfx.projects-api.project_document.created" // Self-published; emails project writers/auditors about the new document
"lfx.projects-api.project_link.created"     // Self-published; emails project writers/auditors about the new link

// Outbound events (published by this service)
"lfx.index.project"                    // Project created/updated/deleted for indexing
"lfx.index.project_settings"           // Settings created/updated/deleted for indexing
"lfx.index.project_link"               // Link created/deleted for indexing
"lfx.index.project_folder"             // Folder created/deleted for indexing
"lfx.index.project_document"           // Document created/deleted for indexing
"lfx.projects-api.project_settings.updated" // Settings changed (before/after snapshot)
"lfx.projects-api.project_document.created" // File document uploaded (events.ProjectDocumentCreatedMessage)
"lfx.projects-api.project_link.created"     // Link added (events.ProjectLinkCreatedMessage)
"lfx.fga-sync.update_access"           // Generic FGA access control updates
"lfx.fga-sync.delete_access"           // Generic FGA access control deletion

// Outbound request/reply (published by this service, awaits a response)
"lfx.email-service.send_email"         // Request to email service for role notifications
"lfx.invite-service.send_invite"       // Request to invite service for non-LFID users
```

### FGA Sync Message Format

The service uses the generic FGA sync handlers for access control. All messages use the `GenericFGAMessage` envelope:

```go
// Update access control (full sync) — fgatypes.GenericAccessData
GenericFGAMessage{
    ObjectType: "project",
    Operation: "update_access",
    Data: GenericAccessData{
        UID: "project-uid",
        Public: true,
        Relations: map[string][]string{
            "writer": []string{"username1", "username2"},
            "auditor": []string{"username3"},
            "meeting_coordinator": []string{"username4"},
            "executive_director": []string{"username5"},
        },
        References: map[string][]string{
            "parent": []string{"project:parent-uid"},
        },
    },
}

// Delete all access control — fgatypes.GenericDeleteData
GenericFGAMessage{
    ObjectType: "project",
    Operation: "delete_access",
    Data: GenericDeleteData{
        UID: "project-uid",
    },
}
```

**Key Points:**

- Relations map user roles to usernames (e.g., `"writer": ["user1", "user2"]`)
- References map object relationships with formatted UIDs (e.g., `"parent": ["project:parent-uid"]`)
- Update operations are full sync - any relations not included will be removed
- Delete operations remove all access control tuples for the resource

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
| `SKIP_ETAG_VALIDATION` | Skip If-Match/ETag revision enforcement on writes (`true` to skip; local dev only) | false | No |
| `LFX_ENVIRONMENT` | Deployment environment (`prod`, `staging`/`stg`, else dev); drives the default self-serve base URL | - | No |
| `LFX_SELF_SERVE_BASE_URL` | Base URL for project links in notification emails | derived from `LFX_ENVIRONMENT` | No |
| `EMAILS_ENABLED` | Gate for outbound role-notification emails to LFID users (`true` to enable) | false | No |
| `INVITES_ENABLED` | Gate for outbound invite requests to non-LFID users (`true` to enable) | false | No |

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

There are two main development setup options documented in DEVELOPMENT.md:

### Option A: Full Platform Setup

For integration testing with complete LFX stack:

- Install lfx-platform Helm chart (includes NATS, Heimdall, OpenFGA, Authelia, Traefik)
- Use `make helm-install-local` with values.local.yaml
- Full authentication and authorization enabled

### Option B: Minimal Setup

For rapid development:

```bash
# Just run NATS locally
docker run -d -p 4222:4222 nats:latest -js

# Create KV stores
nats kv add projects --history=20 --storage=file
nats kv add project-settings --history=20 --storage=file

# Run service with mock auth
export NATS_URL=nats://localhost:4222
export JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL=test-user
make run
```

**Security Note**: Option B bypasses all authentication/authorization - only for local development.

### New Helm Commands

- `make helm-install-local`: Install with local values
- `make helm-restart`: Restart deployment pod
- `make docker-build`: Build Docker image

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
**Solution**: Always include If-Match header in PUT/DELETE requests (server responds with ETag header on GET request)

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

### 3. NATS Event Wire Types (`pkg/events/`)

NATS message payload types that other services consume belong in `pkg/events/`, **not** `internal/`. This lets downstream services (e.g., `lfx-v2-invite-service`) import the canonical struct definitions directly.

- Domain types in `internal/domain/models/` may differ from wire types and can evolve independently.
- Explicit converter functions in `internal/service/converters.go` map from domain → event type before publishing.
- Example: `DomainSettingsToEvent(*models.ProjectSettings) events.ProjectSettings`

**Rule:** if a struct appears in a NATS message payload, it belongs in `pkg/events/`, not `internal/`.

### 4. Request Context

Important context values:

- `request-id`: Unique request identifier
- `authorization`: JWT token from header
- `etag`: ETag value for optimistic concurrency (sent as If-Match header in requests)

### 5. Error Handling

Domain errors are named sentinels in `internal/domain/errors.go`, mapped to HTTP status codes by `handleError` in `cmd/project-api/service_endpoint_project.go`:

- `ErrProjectNotFound` / `ErrDocumentNotFound` / `ErrLinkNotFound` / `ErrFolderNotFound` → 404
- `ErrProjectSlugExists` / `ErrRevisionMismatch` / `ErrDocumentNameExists` / `ErrFolderNameExists` / `ErrFolderNotEmpty` → 409
- `ErrValidationFailed` / `ErrInvalidParentProject` / `ErrInvalidContentType` / `ErrFileTooLarge` / `ErrCannotDeleteNonCrowdfundingProject` → 400
- `ErrInternal` / `ErrUnmarshal` → 500
- `ErrServiceUnavailable` → 503

## Debugging Tips

1. **Enable Debug Logging**: Run with `-d` flag or set `LOG_LEVEL=debug`
2. **Check NATS Messages**: Use `nats sub "lfx.>"` to monitor all messages
3. **Verify KV Data**: Use `nats kv get projects <uid>` to check stored data
4. **HTTP Traces**: Middleware logs all requests with timing
5. **Generated Code**: Check `api/project/v1/gen/` directory for Goa-generated interfaces

## Documentation Structure

The project has a clear documentation hierarchy:

- **README.md**: Project overview, quick start, API endpoints, deployment setup
- **DEVELOPMENT.md**: Comprehensive developer guide with build/test/deploy workflows
- **CLAUDE.md**: AI assistant instructions and technical details (this file)

Key documentation patterns:

- README focuses on getting the service running quickly
- DEVELOPMENT.md covers the full development workflow
- Avoid duplicating content between files - use cross-references instead

## Contributing Guidelines

1. **Design First**: Update Goa design files before implementation
2. **Test Coverage**: Write comprehensive unit tests
3. **Mock External Deps**: Use mocks for repository and message builder
4. **Follow Clean Architecture**: Respect layer boundaries
5. **Update Docs**: Keep documentation current and avoid duplication
6. **Lint Clean**: Ensure `make check` passes

## Resources

- [Goa Framework Docs](https://goa.design/docs/)
- [NATS JetStream Docs](https://docs.nats.io/jetstream)
- [OpenFGA Docs](https://openfga.dev/docs)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
